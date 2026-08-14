package authority

import (
	"errors"
	"os"
	"strings"
	"time"
)

// ActiveForSubject returns the one effective authority grant for a trusted
// subject identity. Multiple effective grants are treated as ambiguous rather
// than silently selecting the broadest or newest authority.
func (s *Store) ActiveForSubject(subjectID string) (*Grant, error) {
	subjectID = strings.TrimSpace(subjectID)
	if subjectID == "" {
		return nil, errors.New("subject_id required")
	}
	grants, err := s.List()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var match *Grant
	for _, grant := range grants {
		if strings.TrimSpace(grant.Envelope.SubjectID) != subjectID {
			continue
		}
		effective, effectiveErr := s.Effective(grant.Envelope.ID, now)
		if effectiveErr != nil {
			continue
		}
		if match != nil {
			return nil, errors.New("multiple effective authority grants for subject")
		}
		match = effective
	}
	if match == nil {
		return nil, os.ErrNotExist
	}
	return match, nil
}
