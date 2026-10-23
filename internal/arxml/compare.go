package arxml

import (
	"fmt"
	"reflect"
	"sort"
)

type Change struct {
	Status string `json:"status"`
	Path   string `json:"path"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}
type Comparison struct {
	Before  string         `json:"before"`
	After   string         `json:"after"`
	Changes []Change       `json:"changes"`
	Summary map[string]int `json:"summary"`
}

// Named children have stable identities; unnamed repeated children keep their order.
func semantic(n *Node) any {
	out := map[string]any{}
	if n.Text != "" {
		out["value"] = n.Text
	}
	if len(n.Attributes) > 0 {
		out["attributes"] = n.Attributes
	}
	if n.Tag == "ARGUMENTS" {
		items := []any{}
		for _, c := range n.Children {
			items = append(items, semantic(c))
		}
		out["arguments"] = items
		return out
	}
	counts := map[string]int{}
	for _, c := range n.Children {
		counts[c.Tag]++
	}
	ord := map[string]int{}
	for _, c := range n.Children {
		key := c.Tag
		if name := c.Value("SHORT-NAME"); name != "" {
			key += "[" + name + "]"
		} else if counts[key] > 1 {
			ord[key]++
			key += fmt.Sprintf("[%d]", ord[key])
		}
		out[key] = semantic(c)
	}
	return out
}
func snapshot(d *Document) map[string]any {
	services := map[string]any{}
	types := map[string]any{}
	for _, s := range d.Services {
		base := map[string]any{"kind": s.Kind}
		properties := map[string]any{}
		for _, c := range s.Definition.Children {
			if !unordered[c.Tag] {
				properties[c.Tag] = semantic(c)
			}
		}
		base["properties"] = properties
		for k, list := range map[string][]Member{"methods": s.Methods, "events": s.Events, "fields": s.Fields, "data_elements": s.DataElements} {
			members := map[string]any{}
			for _, m := range list {
				members[m.Name] = semantic(m.Definition)
			}
			base[k] = members
		}
		deps := map[string]any{}
		for _, dep := range s.Deployments {
			deps[dep.Path] = semantic(dep.Definition)
		}
		base["deployments"] = deps
		services[s.Path] = base
	}
	for _, t := range d.Types {
		types[t.Path] = semantic(t.Definition)
	}
	return map[string]any{"services": services, "types": types}
}
func Compare(a, b *Document) Comparison {
	result := Comparison{Before: a.Source, After: b.Source, Changes: []Change{}, Summary: map[string]int{"added": 0, "removed": 0, "modified": 0}}
	add := func(status, path string, a, b any) {
		result.Changes = append(result.Changes, Change{status, path, a, b})
		result.Summary[status]++
	}
	var diff func(any, any, string)
	diff = func(a, b any, path string) {
		if reflect.DeepEqual(a, b) {
			return
		}
		am, aok := a.(map[string]any)
		bm, bok := b.(map[string]any)
		if aok && bok {
			keys := map[string]bool{}
			for k := range am {
				keys[k] = true
			}
			for k := range bm {
				keys[k] = true
			}
			ordered := []string{}
			for k := range keys {
				ordered = append(ordered, k)
			}
			sort.Strings(ordered)
			for _, k := range ordered {
				av, existsA := am[k]
				bv, existsB := bm[k]
				p := path + "/" + k
				if !existsA {
					add("added", p, nil, bv)
				} else if !existsB {
					add("removed", p, av, nil)
				} else {
					diff(av, bv, p)
				}
			}
		} else {
			add("modified", path, a, b)
		}
	}
	diff(snapshot(a), snapshot(b), "")
	return result
}
