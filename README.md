# deps
Dependency injection with Go and generics. Featuring lazy loading, lifetime control, pointer usage detection, hierarchal scopes, resource freeing, dynamic providers, and dynamic types.


```go
package main

import github.com/ClickerMonkey/deps

type Port int
type Env struct {
  Connection string
}
type Database struct {
  Query func(sql string) []any
}
type UserPreferences struct {
  Name string
}
type Param[V any] struct {
  Value V
}
// Dynamic values, essential for generics
func (p Param[V]) ProvideDynamic(scope *Scope, typ reflect.Type) (any, error) {
  val := reflect.New(typ).Interface()
  // do stuff
  return val, nil
}

func main() {
  // Set values or provide functions
  deps.Set(Port(8080))

  // Globally provided value
  deps.Provide(deps.Provider[Env]{
    Create: func(scope *deps.Scope) (*Env, error) {
      // TODO load environment values and return env
      return &Env{}, nil
    },
  })
  // Value that lives on scope so it can be properly freed
  deps.Provide(deps.Provider[Database]{
    Lifetime: deps.LifetimeScope,
    Create: func(scope *deps.Scope) (*Database, error) {
      env, err := deps.GetScoped[Env](scope)
      if err != nil {
        return nil, err
      }
      // TODO create connection from env.Connection
      return &Database{}, nil
    },
    Free: func(scope *deps.Scope, db *Database) error {
      // TODO close connection
    },
  })
  // Value that exists globally but is notified when its potentially modified.
  deps.Provide(deps.Provider[UserPreferences]{
    Create: func(scope *deps.Scope) (*UserPreferences, error) {
      return &UserPreferences{Name: "ClickerMonkey"}, nil
    },
    AfterPointerUse: func(scope *deps.Scope, prefs *UserPreferences) error {
      // save user preferences, a pointer was requested to change its state.
      return nil
    },
  })

  // Invoke global function
  deps.Invoke(func(port Port, param Param[string]) {
    // do stuff with port.
    // param was dynamically created by an instance of its type, great for generics
  })

  // A child scope from Global
  s := deps.New()
  env, _ := deps.Get[Env]() // get env from global scope
  db, _ := deps.GetScoped[Database](s) // gets database connection and stores it in this scope
  s.Free() // free values in this scope

  // Store a value directly on this scope
  s.Set("Phil") // newName below
  // Also do a custom provider for this scope only
  deps.ProvideScoped(s, deps.Provider[int]{
    Create: func(scope *deps.Scope) (*int, error) {
      age := 33
      return &age, nil
    }
  })

  // Invoke a function and provide it with values from the scope.
  s.Invoke(func(env Env, prefs *UserPreferences, newName string, age int) {
    prefs.Name = newName // would trigger a notification above
  })
  
  // We can also dynamically provide other values that are not set in the scope or its ancestors or
  // are associated with any providers or implements deps.Dynamic.
  s.DynamicProvider = func(typ reflect.Type, scope *Scope) (any, error) {
    // generate value of typ if supported, otherwise return nil, nil
    return nil, nil
  }

  // For good measure
  deps.Global().Free()
}

```
