package ownership

import (
	"reflect"
	"strings"
	"testing"
)

// ADR-0017 §3 as a structural guarantee: no single blended score is
// representable anywhere in this package's API. The ratio is an integer
// pair per finding kind, so a float field or a float-returning method is
// exactly the shape a blend would take, and neither may exist, whatever
// it gets called.
func TestNoBlendedScoreIsRepresentable(t *testing.T) {
	roots := []reflect.Type{
		reflect.TypeOf(Estate{}),
		reflect.TypeOf(Tree{}),
		reflect.TypeOf(Rollup{}),
		reflect.TypeOf(Score{}),
		reflect.TypeOf(Finding{}),
		reflect.TypeOf(RoutedFinding{}),
	}
	forbidden := []string{"blended", "overall", "percent", "aggregate", "total"}

	var walkType func(typ reflect.Type, path string, seen map[reflect.Type]bool)
	walkType = func(typ reflect.Type, path string, seen map[reflect.Type]bool) {
		for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		switch typ.Kind() {
		case reflect.Float32, reflect.Float64:
			t.Errorf("%s is a float: a blended score in the making", path)
			return
		case reflect.Struct:
		default:
			return
		}
		if seen[typ] {
			return
		}
		seen[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Type.Kind() == reflect.Float32 || f.Type.Kind() == reflect.Float64 {
				t.Errorf("%s.%s is a float: a blended score in the making", path, f.Name)
			}
			lower := strings.ToLower(f.Name)
			for _, bad := range forbidden {
				if strings.Contains(lower, bad) {
					t.Errorf("%s.%s: no field may carry a %s score", path, f.Name, bad)
				}
			}
			walkType(f.Type, path+"."+f.Name, seen)
		}
	}

	seen := map[reflect.Type]bool{}
	for _, typ := range roots {
		walkType(typ, typ.Name(), seen)

		// Methods too: a Percent() or Overall() helper would be the same
		// blend behind a function call.
		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			lower := strings.ToLower(m.Name)
			for _, bad := range forbidden {
				if strings.Contains(lower, bad) {
					t.Errorf("%s.%s: no method may offer a %s score", typ.Name(), m.Name, bad)
				}
			}
			for o := 0; o < m.Type.NumOut(); o++ {
				out := m.Type.Out(o)
				if out.Kind() == reflect.Float32 || out.Kind() == reflect.Float64 {
					t.Errorf("%s.%s returns a float: a blended score in the making", typ.Name(), m.Name)
				}
			}
		}
	}
}

// The authored set is exactly ADR-0016's, and a collector is deliberately
// outside it.
func TestAuthoredSetMatchesADR0016(t *testing.T) {
	authored := []ObjectKind{
		KindComponent, KindBlueprint, KindTier, KindHop,
		KindPath, KindService, KindRequirement, KindExemption,
	}
	for _, k := range authored {
		if !k.Authored() {
			t.Errorf("%s must be in the authored set", k)
		}
	}
	if KindCollector.Authored() {
		t.Error("a collector must never be authored or ownable")
	}
	if ObjectKind("dashboard").Authored() {
		t.Error("an unknown kind must not read as authored")
	}
}
