package deps

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var ErrMissingCreate = errors.New("provider missing create function")
var ErrNoProvider = errors.New("no provider exists for the given type")
var ErrNotPointer = errors.New("only pointers can be set on a scope")
var ErrNotFunc = errors.New("only funcs can be invoked")
var ErrInvalidValue = errors.New("invalid argument for invoke")

var global *Scope = new(nil)

// Returns the global scope. All scopes created with New() has this scope as the parent.
// The global Set, Get, Provide, Invoke, & Hydrate functions operate based on providers
// given to this global scope. All child scopes can return values created globally depending
// on the provided lifetime.
func Global() *Scope {
	return global
}

// A dynamic provider if a requested type does not have value or provider.
// If the value returned is not the expected type a ErrNoProvider will be thrown.
type DynamicProvider func(typ reflect.Type, scope *Scope) (any, error)

// If a type to be provided doesn't have a provider but implements this interface
// the type itself becomes a provider. This is especially useful for types with
// generics and the types are not known ahead of time or there are too many to be
// individually provided.
type Dynamic interface {
	// Given the scope its trying to be created in, the specific type, try to populate
	// the instance of this value. If there was an error it will be passed up through
	// the invokation or the hydration request.
	ProvideDynamic(scope *Scope) error
}

// The reflection type for Dynamic.
var dynamicType = TypeOf[Dynamic]()

// Given a type it returns an instance of it if it implements the Dynamic interface.
// If it does not, nil is returned.
func GetDynamic(typ reflect.Type) Dynamic {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	ptr := reflect.PointerTo(typ)
	if !ptr.Implements(dynamicType) {
		return nil
	}
	val := reflect.New(typ).Interface()
	return val.(Dynamic)
}

// Sets a constant value on the global scope.
func Set[V any](value *V) {
	SetScoped(global, value)
}

// Sets a constant value on the given scope.
func SetScoped[V any](scope *Scope, value *V) {
	key := TypeOf[V]()
	scope.instances[key] = value
}

// Returns a constant value from the global scope.
func Get[V any]() (*V, error) {
	return GetScoped[V](global)
}

// Returns a constant value from the given scope and an error if there was an error
// trying to create the value. If a value with the expected type does not exist
// in this scope or its parent and a dynamic provider is defined that is called.
// If the result of the dynamic pointer is type V or *V then it's returned without error.
func GetScoped[V any](scope *Scope) (*V, error) {
	key := TypeOf[V]()
	if instance, exists := scope.instances[key]; exists {
		return instance.(*V), nil
	}
	provider := scope.providers[key]
	if provider == nil {
		if scope.parent != nil {
			par, err := GetScoped[V](scope.parent)
			if err == nil || err != ErrNoProvider {
				return par, err
			}
		}
		dynamic := GetDynamic(key)
		if dynamic != nil {
			err := dynamic.ProvideDynamic(scope)
			if err != nil {
				return nil, err
			}
			dyn := any(dynamic)
			if val, ok := dyn.(*V); ok {
				return val, nil
			}
			if val, ok := dyn.(V); ok {
				return &val, nil
			}
		}
		if scope.Dynamic != nil {
			dyn, err := scope.Dynamic(key, scope)
			if err != nil {
				return nil, err
			}
			if val, ok := dyn.(*V); ok {
				return val, nil
			}
			if val, ok := dyn.(V); ok {
				return &val, nil
			}
		}
		return nil, ErrNoProvider
	}
	instance, err := provider.(*providerLink[V]).provider.Create(scope)
	if err != nil {
		return nil, err
	}
	scope.instances[key] = instance
	return instance, nil
}

// Registers a provider on the global scope. A Provider can specify lifetime rules and can handle
// lazily creating new values and freeing them when their lifetime expires. A provider can also
// be notified about a potential value change when Invoke is called with a function which accepts
// the pointer argument.
func Provide[V any](provider Provider[V]) {
	ProvideScoped(global, provider)
}

// Registers a provider on the given scope. A Provider can specify lifetime rules and can handle
// lazily creating new values and freeing them when their lifetime expires. A provider can also
// be notified about a potential value change when Invoke is called with a function which accepts
// the pointer argument.
func ProvideScoped[V any](scoped *Scope, provider Provider[V]) {
	key := TypeOf[V]()
	scoped.providers[key] = &providerLink[V]{
		key:      key,
		provider: provider,
	}
}

// Invokes a function passing provided values from the global scope as arguments. Any argument
// types that do not have a constant or provider will get their default value.
func Invoke(fn any) (Result, error) {
	return global.Invoke(fn)
}

// Given a pointer to any value this will traverse it using the global scope and when it finds
// types of provided values it updates them.
func Hydrate(value any) error {
	return global.Hydrate(value)
}

// Returns the reflect.Type of V
func TypeOf[V any]() reflect.Type {
	return reflect.TypeOf((*V)(nil)).Elem()
}

// How long values should last in a scope.
type Lifetime int

const (
	// The value should last forever, or until scope.Free() is called. If a provider or value
	// is not explicitly set on the current scope it will reach out to the parent scopes all the way
	// to the global scope, and prefer to place values on the global scope since they desire to
	// last forever.
	LifetimeForever Lifetime = iota
	// The value will be created on the given scope and freed when scope.Free() is called.
	LifetimeScope
	// The value will be created for invoke or hydration but immediately freed after that.
	LifetimeOnce
)

type link interface {
	lifetime() Lifetime
	get(scope *Scope) (any, error)
	afterPointerUse(scope *Scope) error
	free(scope *Scope) error
}

type providerLink[V any] struct {
	provider Provider[V]
	key      reflect.Type
}

func (link *providerLink[V]) lifetime() Lifetime {
	return link.provider.Lifetime
}

func (link *providerLink[V]) get(scope *Scope) (any, error) {
	value := scope.instances[link.key]
	if value == nil {
		if link.provider.Create == nil {
			return value, ErrMissingCreate
		}
		created, err := link.provider.Create(scope)
		if err != nil {
			return created, err
		}
		scope.instances[link.key] = created
		value = created
	}
	return value.(*V), nil
}

func (link *providerLink[V]) afterPointerUse(scope *Scope) error {
	if link.provider.AfterPointerUse != nil {
		value := scope.instances[link.key].(*V)
		return link.provider.AfterPointerUse(scope, value)
	}
	return nil
}

func (link *providerLink[V]) free(scope *Scope) error {
	var err error
	if link.provider.Free != nil {
		value := scope.instances[link.key].(*V)
		err = link.provider.Free(scope, value)
	}
	delete(scope.instances, link.key)
	return err
}

type Provider[V any] struct {
	Lifetime        Lifetime
	Create          func(scope *Scope) (*V, error)
	AfterPointerUse func(scope *Scope, value *V) error
	Free            func(scope *Scope, value *V) error
}

type Scope struct {
	Dynamic DynamicProvider

	parent    *Scope
	providers map[reflect.Type]link
	instances map[reflect.Type]any
}

// Creates a new scope with the global scope as the parent.
func New() *Scope {
	return new(global)
}

func new(parent *Scope) *Scope {
	return &Scope{
		parent:    parent,
		providers: make(map[reflect.Type]link),
		instances: make(map[reflect.Type]any),
	}
}

// Returns this scope's parent.
func (scope *Scope) Parent() *Scope {
	return scope.parent
}

// Returns a child to this scope.
func (scope *Scope) Spawn() *Scope {
	return new(scope)
}

// Sets a value on this scope.
func (scope *Scope) Set(value any) error {
	key := reflect.TypeOf(value)
	if key.Kind() != reflect.Pointer {
		ptr := reflect.New(key)
		ptr.Elem().Set(reflect.ValueOf(value))
		scope.instances[key] = ptr.Interface()
	} else {
		scope.instances[key.Elem()] = value
	}
	return nil
}

// Gets a value from this scope with the given type and potentially returns an error.
// If it doesn't exist on this scope a provider is searched through the parent scopes.
// If the provider has a lifetime of forever its created on the deepest scope, otherwise
// scope and once lifetime values are stored in this scope.
func (scope *Scope) Get(key reflect.Type) (any, error) {
	if instance, exists := scope.instances[key]; exists {
		return instance, nil
	}
	deepLink := scope.getLink(key)
	if deepLink != nil && deepLink.lifetime() == LifetimeScope {
		return deepLink.get(scope)
	}
	link := scope.providers[key]
	if link == nil {
		if scope.parent != nil {
			par, err := scope.parent.Get(key)
			if err == nil || err != ErrNoProvider {
				return par, err
			}
		}
		dynamic := GetDynamic(key)
		if dynamic != nil {
			err := dynamic.ProvideDynamic(scope)
			if err != nil {
				return nil, err
			}
			return dynamic, nil
		}
		if scope.Dynamic != nil {
			dyn, err := scope.Dynamic(key, scope)
			if err != nil {
				return nil, err
			}
			if dyn != nil {
				return dyn, nil
			}
		}
		return nil, ErrNoProvider
	}
	return link.get(scope)
}

// Returns a provider link for the given type by looking in this scope and then parent scopes
// until it finds a provider.
func (scope *Scope) getLink(key reflect.Type) link {
	if l, exists := scope.providers[key]; exists {
		return l
	} else if scope.parent != nil {
		return scope.parent.getLink(key)
	}
	return nil
}

// Frees all values in this scope with a lifetime of once.
func (scope *Scope) FreeOnce() error {
	multi := multiError{}
	for key := range scope.instances {
		if link := scope.getLink(key); link != nil {
			if link.lifetime() == LifetimeOnce {
				err := link.free(scope)
				if err != nil {
					multi.errors = append(multi.errors, err)
				}
			}
		} else {
			delete(scope.instances, key)
		}
	}
	if len(multi.errors) > 0 {
		return multi
	}
	return nil
}

// Frees all values in this scope.
func (scope *Scope) Free() error {
	multi := multiError{}
	for key := range scope.instances {
		if link := scope.getLink(key); link != nil {
			err := link.free(scope)
			if err != nil {
				multi.errors = append(multi.errors, err)
			}
		} else {
			delete(scope.instances, key)
		}
	}
	if len(multi.errors) > 0 {
		return multi
	}
	return nil
}

// Given a pointer to any value this will traverse it using this scope and when it finds
// types of provided values it updates them. Once the hydrated values are doing being used
// scope.FreeOnce() should be called.
func (scope *Scope) Hydrate(value any) error {
	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Pointer {
		return ErrNotPointer
	}
	err := scope.hydrateValue(val)
	return err
}

// Hydrates a pointer to a value.
func (scope *Scope) hydrateValue(ptr reflect.Value) error {
	key := ptr.Type().Elem()
	val, err := scope.Get(key)
	if err != ErrNoProvider {
		if err == nil && ptr.Elem().CanSet() {
			ptr.Elem().Set(reflect.ValueOf(val).Elem())
		}
		return err
	}
	inner := ptr.Elem()

	switch inner.Kind() {
	case reflect.Chan, reflect.Slice, reflect.Func, reflect.Pointer, reflect.Interface:
		if inner.IsNil() {
			return nil
		}
	}

	switch inner.Kind() {
	case reflect.Array, reflect.Slice:
		n := inner.Len()
		for i := 0; i < n; i++ {
			item := inner.Index(i)
			if item.CanAddr() {
				err := scope.hydrateValue(item.Addr())
				if err != nil {
					return err
				}
			}
		}
	case reflect.Struct:
		n := inner.NumField()
		for i := 0; i < n; i++ {
			field := inner.Field(i)
			if field.CanAddr() {
				err := scope.hydrateValue(field.Addr())
				if err != nil {
					return err
				}
			}
		}
	case reflect.Map:
		keys := inner.MapKeys()
		for _, key := range keys {
			value := inner.MapIndex(key)
			newValue := reflect.New(value.Type())
			err := scope.hydrateValue(newValue)
			if err != nil {
				return err
			}
			inner.SetMapIndex(key, newValue.Elem())
		}
	}
	return nil
}

// Returns a hydrated value of the given type.
func (scope *Scope) hydrateType(key reflect.Type) (reflect.Value, error) {
	if key.Kind() == reflect.Pointer {
		val, err := scope.Get(key.Elem())
		if err != ErrNoProvider {
			return reflect.ValueOf(val), err
		}
	}
	val := reflect.New(key)
	err := scope.hydrateValue(val)
	return val.Elem(), err
}

// Invokes the given function by providing arguments of the requested types with
// values found or provided in this scope and its parents. If the function has a pointer
// argument to a provided type and the provider has a AfterPointerUse defined it will
// be called after the function returns. If any values were created on this scope with
// a lifetime of once they will be freed after the function returns.
func (scope *Scope) Invoke(fn any) (Result, error) {
	fnValue := reflect.ValueOf(fn)
	fnType := reflect.TypeOf(fn)

	if fnType.Kind() != reflect.Func {
		return nil, ErrNotFunc
	}

	n := fnType.NumIn()
	args := make([]reflect.Value, n)
	for i := 0; i < n; i++ {
		argValue, err := scope.hydrateType(fnType.In(i))
		if err != nil {
			return nil, err
		}
		if !argValue.IsValid() {
			return nil, ErrInvalidValue
		}
		args[i] = argValue
	}

	resultsReflect := fnValue.Call(args)

	for i := 0; i < n; i++ {
		argValue := args[i]
		if argValue.Kind() == reflect.Pointer {
			key := argValue.Type().Elem()
			link := scope.providers[key]
			if link != nil {
				err := link.afterPointerUse(scope)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	scope.FreeOnce()

	results := make([]any, len(resultsReflect))
	for i := 0; i < len(results); i++ {
		results[i] = resultsReflect[i].Interface()
	}
	return Result(results), nil
}

type Result []any

func (r Result) Err() error {
	for _, result := range r {
		if err, ok := result.(error); ok {
			return err
		}
	}
	return nil
}

type multiError struct {
	errors []error
}

var _ error = &multiError{}

func (e multiError) Error() string {
	n := len(e.errors)
	if n == 1 {
		return e.errors[0].Error()
	} else {
		errors := make([]string, n)
		for i := 0; i < n; i++ {
			errors[i] = e.errors[i].Error()
		}
		return fmt.Sprintf("multiple errors: %s", strings.Join(errors, ", "))
	}
}
