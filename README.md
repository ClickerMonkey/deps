# deps
Dependency injection with Go and generics. Featuring lazy loading, lifetime control, pointer usage detection, a hierarchy, and resource freeing.


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

func main() {
  port := Port(8080)

  // Set values or provide functions
  deps.Set(&port)

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

  // Invoke global function
  deps.Invoke(func(port Port) {
    // do stuff with port.
  })

  // For good measure
  deps.Global().Free()
}

```