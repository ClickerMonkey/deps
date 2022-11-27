package deps

import (
	"fmt"
	"reflect"
	"testing"
)

func TestSetFunc(t *testing.T) {
	type Point struct{ X, Y int }

	s := New()
	SetScoped(s, &Point{1, 2})

	p, _ := GetScoped[Point](s)

	if p.X != 1 || p.Y != 2 {
		t.Errorf("There was a problem with SetScoped & GetScoped")
	}
}

func TestSetMethod(t *testing.T) {
	type Point struct{ X, Y int }

	s := New()
	s.Set(&Point{1, 2})

	p, _ := GetScoped[Point](s)

	if p.X != 1 || p.Y != 2 {
		t.Errorf("There was a problem with Set & GetScoped")
	}
}

func TestSetGetMethods(t *testing.T) {
	type Point struct{ X, Y int }

	s := New()
	s.Set(&Point{1, 2})
	p, _ := s.Get(reflect.TypeOf(Point{}))
	q := p.(*Point)

	if q.X != 1 || q.Y != 2 {
		t.Errorf("There was a problem with Set & GetScoped")
	}
}

func TestSetInvokeValue(t *testing.T) {
	type Port int

	port := Port(8080)

	s := New()
	s.Set(&port)

	given := Port(0)

	s.Invoke(func(p Port) {
		given = p
	})

	if given != 8080 {
		t.Errorf("Invoked failed to pass correct port")
	}
}

func TestProvideInvokeValue(t *testing.T) {
	type Port int

	s := New()
	ProvideScoped(s, Provider[Port]{
		Create: func(scope *Scope) (*Port, error) {
			port := Port(8080)
			return &port, nil
		},
	})

	given := Port(0)

	s.Invoke(func(p Port) {
		given = p
	})

	if given != 8080 {
		t.Errorf("Invoked failed to pass correct port")
	}
}

func TestSetInvokePointer(t *testing.T) {
	type Port int

	port := Port(8080)

	s := New()
	s.Set(&port)
	s.Invoke(func(p *Port) {
		*p = Port(4040)
	})

	if port != 4040 {
		t.Errorf("Invoked failed to update port")
	}
}

func TestInvokeEmpty(t *testing.T) {
	invoked := false
	Invoke(func(a []struct{}, b map[string]struct{}, c int, d *string) {
		invoked = true
	})

	if !invoked {
		t.Errorf("Invoke failed to call with empty values")
	}
}

func TestAfterUse(t *testing.T) {
	type Port int

	var afterUse Port

	s := New()
	ProvideScoped(s, Provider[Port]{
		Create: func(scope *Scope) (*Port, error) {
			port := Port(8080)
			return &port, nil
		},
		AfterPointerUse: func(scope *Scope, value *Port) error {
			afterUse = *value
			return nil
		},
	})

	s.Invoke(func(p *Port) {
		if afterUse != Port(0) {
			t.Errorf("After use should not be populated yet")
		}
	})

	if afterUse != Port(8080) {
		t.Errorf("After use does not have correct value")
	}
}

func TestStructValue(t *testing.T) {
	type Port int
	type Env struct {
		Port Port
	}

	port := Port(8080)
	s := New()
	s.Set(&port)

	s.Invoke(func(e Env) {
		if e.Port != Port(8080) {
			t.Errorf("Hydrating env failed")
		}
	})
}

func TestStructEmbeddedValue(t *testing.T) {
	type Port int
	type Env struct {
		Port
	}

	port := Port(8080)
	s := New()
	s.Set(&port)

	s.Invoke(func(e Env) {
		if e.Port != Port(8080) {
			t.Errorf("Hydrating env failed")
		}
	})
}

func TestArray(t *testing.T) {
	type Port int
	type Env struct {
		Ports [1]Port
	}

	s := New()
	s.Set(Port(8080))

	s.Invoke(func(e Env) {
		if e.Ports[0] != Port(8080) {
			t.Errorf("Hydrating array port failed")
		}
	})
}

type Request interface {
	GetBody() any
	SetStatus(status int)
}
type MyRequest[B any] struct {
	body   B
	status int
}

var _ Request = &MyRequest[string]{}

func (mr MyRequest[B]) GetBody() any {
	return mr.body
}
func (mr *MyRequest[B]) SetStatus(status int) {
	mr.status = status
}

func TestDynamic(t *testing.T) {
	intType := TypeOf[int]()
	stringType := TypeOf[string]()
	requestType := TypeOf[Request]()

	if fmt.Sprintf("%v", intType) != "int" {
		t.Errorf("Error creating int type")
	}
	if fmt.Sprintf("%v", stringType) != "string" {
		t.Errorf("Error creating string type")
	}
	if fmt.Sprintf("%v", requestType) != "deps.Request" {
		t.Errorf("Error creating interface type")
	}

	// fmt.Printf("%v, %v, %v\n", intType, stringType, requestType)

	s := New()
	s.Dynamic = func(typ reflect.Type, scope *Scope) (any, error) {
		if typ == intType {
			return 42, nil
		} else if typ == stringType {
			return "abc", nil
		} else if typ == requestType {
			return &MyRequest[string]{body: "My Body"}, nil
		} else if reflect.PointerTo(typ).Implements(requestType) {
			val := reflect.New(typ).Interface() // MyRequest[V]
			if req, ok := val.(Request); ok {
				req.SetStatus(200)
			}
			return val, nil
		}
		return nil, nil
	}

	i, _ := GetScoped[int](s)
	if i == nil || *i != 42 {
		t.Errorf("dynamic int is not 42: %v", i)
	}

	str, _ := GetScoped[string](s)
	if str == nil || *str != "abc" {
		t.Errorf("dynamic string is not abc: %v", str)
	}

	r1, _ := GetScoped[Request](s)
	if r1 == nil || (*r1).GetBody() != "My Body" {
		t.Errorf("dynamic request is not My Body: %+v", r1)
	}

	r2, _ := GetScoped[MyRequest[bool]](s)
	if r2 == nil || (*r2).GetBody() != false {
		t.Errorf("dynamic my request is not false: %+v", r2)
	}
	if r2.status != 200 {
		t.Errorf("dynamic my request does not have a status of 200: %+v", r2.status)
	}
}

type Gen[V any] struct {
	Value V
}

var _ Dynamic = &Gen[int]{}

func (g *Gen[V]) ProvideDynamic(scope *Scope) error {
	if gint, ok := any(g).(*Gen[int]); ok {
		gint.Value = 42
	}
	if gstr, ok := any(g).(*Gen[string]); ok {
		gstr.Value = "Hello World"
	}
	return nil
}

func TestDynamicValue(t *testing.T) {
	s := New()
	gint, _ := GetScoped[Gen[int]](s)

	if gint == nil {
		t.Errorf("Failure creating Dynamic Gen[int]")
		return
	}

	if gint.Value != 42 {
		t.Errorf("Failure setting value of Gen[int]")
	}

	gstr, _ := GetScoped[Gen[string]](s)

	if gstr == nil {
		t.Errorf("Failure creating Dynamic Gen[gstrin]")
		return
	}

	if gstr.Value != "Hello World" {
		t.Errorf("Failure setting value of Gen[string]")
	}

	s.Invoke(func(gint Gen[int]) {
		if gint.Value != 42 {
			t.Errorf("Failure passing argument of Gen[int]")
		}
	})
}
