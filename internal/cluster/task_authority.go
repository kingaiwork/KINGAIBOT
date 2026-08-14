package cluster

import (
	"errors"
	"strings"
	"sync"
)

// TaskAuthorityResolver returns the authority already bound to a durable task
// by trusted runtime code. It deliberately accepts only task identity, not
// model-supplied authority arguments.
type TaskAuthorityResolver interface {
	AuthorityForTask(taskID string) (string, error)
}

var coordinatorTaskAuthority sync.Map

func (c *Coordinator) SetTaskAuthorityResolver(resolver TaskAuthorityResolver) error {
	if c == nil {
		return errors.New("cluster coordinator required")
	}
	if resolver == nil {
		coordinatorTaskAuthority.Delete(c)
		return nil
	}
	coordinatorTaskAuthority.Store(c, resolver)
	return nil
}

func (c *Coordinator) authorityForTask(taskID string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", errors.New("task identity required for authority-bound cluster execution")
	}
	v, ok := coordinatorTaskAuthority.Load(c)
	if !ok {
		return "", errors.New("task authority resolver is not configured")
	}
	resolver, ok := v.(TaskAuthorityResolver)
	if !ok || resolver == nil {
		return "", errors.New("task authority resolver is invalid")
	}
	return resolver.AuthorityForTask(taskID)
}
