package deps

import (
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
