package gg

import "path/filepath"

// Git discovery no longer registers the calling worktree as a session. Keep
// running legacy sources addressable until they stop, then retire only records.
func (p *Project) currentState(_ string) (*State, error) {
	state, err := p.load()
	if err != nil {
		return nil, err
	}
	hasSources := false
	for _, t := range state.Tasks {
		hasSources = hasSources || t.Protected
	}
	if !hasSources {
		return state, nil
	}
	err = p.transaction(func(s *State) error {
		state = s
		snap, err := p.checkedSnapshot()
		if err != nil {
			// Unknown server state is not evidence that a source can be retired.
			return nil
		}
		kept := make([]*Task, 0, len(s.Tasks))
		for _, t := range s.Tasks {
			if !t.Protected || len(snap.task(t.ID).live()) > 0 {
				kept = append(kept, t)
			}
		}
		if len(kept) == len(s.Tasks) {
			return nil
		}
		s.Tasks = kept
		return atomicJSON(filepath.Join(p.Dir, "state.json"), s)
	}, false)
	return state, err
}
