package formula

import "fmt"

// Validate checks a Formula for structural correctness. It verifies:
// name is present, at least one step exists, no duplicate step IDs, all
// needs refs exist, and no dependency cycles (Kahn's algorithm).
func Validate(f *Formula) error {
	if f.Name == "" {
		return fmt.Errorf("formula name is required")
	}
	if len(f.Steps) == 0 {
		return fmt.Errorf("formula %q has no steps", f.Name)
	}

	ids := make(map[string]bool)
	for _, s := range f.Steps {
		if ids[s.ID] {
			return fmt.Errorf("formula %q has duplicate step ID %q", f.Name, s.ID)
		}
		ids[s.ID] = true
	}

	for _, s := range f.Steps {
		for _, need := range s.Needs {
			if !ids[need] {
				return fmt.Errorf("step %q needs unknown step %q", s.ID, need)
			}
		}
	}

	// Cycle detection via Kahn's algorithm.
	inDegree := make(map[string]int)
	dependents := make(map[string][]string)
	for _, s := range f.Steps {
		if _, ok := inDegree[s.ID]; !ok {
			inDegree[s.ID] = 0
		}
		for _, need := range s.Needs {
			inDegree[s.ID]++
			dependents[need] = append(dependents[need], s.ID)
		}
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, dep := range dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if visited != len(f.Steps) {
		return fmt.Errorf("formula %q has a dependency cycle", f.Name)
	}

	return nil
}
