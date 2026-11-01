package omnicredential

import "slices"

// Exposes returns every credential a holder of id can reach, directly or
// through others: its DependsOn and Reaches, and theirs, sorted. It does
// not include id.
func (r *Registry) Exposes(id string) []string {
	edges := map[string][]string{}
	for _, c := range r.Credentials {
		edges[c.ID] = append(slices.Clone(c.DependsOn), c.Reaches...)
	}
	return walk(id, edges)
}

// ExposedBy returns every credential whose holder can reach id, directly
// or through others, sorted. It does not include id.
func (r *Registry) ExposedBy(id string) []string {
	edges := map[string][]string{}
	for _, c := range r.Credentials {
		for _, to := range append(slices.Clone(c.DependsOn), c.Reaches...) {
			edges[to] = append(edges[to], c.ID)
		}
	}
	return walk(id, edges)
}

func walk(from string, edges map[string][]string) []string {
	seen := map[string]bool{from: true}
	queue := []string{from}
	var out []string
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, m := range edges[n] {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
				queue = append(queue, m)
			}
		}
	}
	slices.Sort(out)
	return out
}
