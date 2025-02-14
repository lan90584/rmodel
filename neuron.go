package rModel

import (
	"fmt"

	"github.com/rs/zerolog"
	"github.com/Rovanta/rmodel/core"
	"github.com/Rovanta/rmodel/internal/errors"
	"github.com/Rovanta/rmodel/internal/utils"
	"github.com/Rovanta/rmodel/processor"
)

func newNeuron(p processor.Processor) *neuron {
	n := &neuron{
		id:            utils.GenIDShort(),
		labels:        make(map[string]string),
		processor:     p,
		triggerGroups: make(triggerGroups),
		castGroups:    make(castGroups),
		selector:      &processor.DefaultSelector{},
	}

	return n
}

func newEndNeuron() *neuron {
	n := &neuron{
		id:            core.EndNeuronID,
		labels:        make(map[string]string),
		processor:     &processor.EmptyProcessor{},
		triggerGroups: make(triggerGroups),
	}

	return n
}

type neuron struct {
	// ID
	id string
	// labels
	labels map[string]string
	// processor
	processor processor.Processor
	// Trigger group, the trigger group is used to control the trigger conditions of Neuron
	// key: group ID, value: list of link ID
	triggerGroups triggerGroups
	// Propagation group, the propagation group is used to control the propagation relationship between Neuron
	// key: group ID/Name, value: map of link ID
	castGroups castGroups
	// After neuron runs successfully, use Selector to decide which propagation group to transmit to.
	selector processor.Selector
}

func (n *neuron) deepCopy() *neuron {
	return &neuron{
		id:            n.id,
		labels:        utils.LabelsDeepCopy(n.labels),
		processor:     n.processor,
		triggerGroups: n.triggerGroups.deepCopy(),
		castGroups:    n.castGroups.deepCopy(),
		selector:      n.selector,
	}
}

func (n *neuron) MarshalZerologObject(e *zerolog.Event) {
	e.Str("id", n.id).
		Interface("labels", n.labels).
		Interface("triggerGroups", n.triggerGroups).
		Interface("castGroups", n.castGroups.format())
}

type castGroups map[string]map[string]struct{}

func (cgs castGroups) deepCopy() castGroups {
	newMap := make(castGroups)

	for key, value := range cgs {
		newInnerMap := make(map[string]struct{})
		for innerKey, innerValue := range value {
			newInnerMap[innerKey] = innerValue
		}
		newMap[key] = newInnerMap
	}

	return newMap
}

func (cgs castGroups) format() map[string][]string {
	newMap := make(map[string][]string)

	for key, value := range cgs {
		var newSlice []string
		for innerKey := range value {
			newSlice = append(newSlice, innerKey)
		}

		newMap[key] = newSlice
	}

	return newMap
}

type triggerGroups map[string][]string

func (tgs triggerGroups) deepCopy() triggerGroups {
	newGs := make(triggerGroups)

	for key, value := range tgs {
		newSlice := make([]string, len(value))
		copy(newSlice, value)
		newGs[key] = newSlice
	}

	return newGs
}

func (n *neuron) GetID() string {
	return n.id
}

func (n *neuron) GetLabels() map[string]string {
	return n.labels
}

func (n *neuron) GetProcessor() processor.Processor {
	return n.processor
}

func (n *neuron) GetSelector() processor.Selector {
	return n.selector
}

func (n *neuron) ListInLinkIDs() []string {
	linkMap := make(map[string]struct{})
	for _, group := range n.triggerGroups {
		for _, l := range group {
			linkMap[l] = struct{}{}
		}
	}
	links := make([]string, 0, len(linkMap))
	for l, _ := range linkMap {
		links = append(links, l)
	}

	return links
}

func (n *neuron) ListOutLinkIDs() []string {
	linkMap := make(map[string]struct{})
	for _, group := range n.castGroups {
		for l, _ := range group {
			linkMap[l] = struct{}{}
		}
	}
	links := make([]string, 0, len(linkMap))
	for l, _ := range linkMap {
		links = append(links, l)
	}

	return links
}

func (n *neuron) ListTriggerGroups() map[string][]string {
	return n.triggerGroups.deepCopy()
}

func (n *neuron) ListCastGroups() map[string][]string {
	return n.castGroups.format()
}

func (n *neuron) SetLabels(labels map[string]string) {
	n.labels = labels
}

// After AddTriggerGroup in-link is connected to neuron, it forms a group by default, that is, an in-link is divided into a trigger group.
// In other words, any in-link can trigger neuron by default.
// AddTriggerGroup is used to put specified links into the same trigger group.
// If the newly divided trigger group contains the existing trigger group, the existing trigger group will be removed.
// If the newly divided trigger group is included in the existing trigger group, the newly divided group will not be created.
// Because only the largest trigger condition needs to be defined, smaller trigger conditions will be included. For example: when {A,B,C} is satisfied, {A,B} must be satisfied.
func (n *neuron) AddTriggerGroup(links ...core.Link) error {
	if len(links) == 0 {
		return nil
	}
	for _, l := range links {
		if !n.hasInLink(l.GetID()) {
			return errors.ErrInLinkNotFound(l.GetID(), n.GetID())
		}
	}

	newGroup := make([]string, 0)
	for _, l := range links {
		newGroup = append(newGroup, l.GetID())
	}

	for key, group := range n.triggerGroups {
		if utils.SlicesContains(group, newGroup) {
			return nil
		}
		if utils.SlicesContains(newGroup, group) {
			delete(n.triggerGroups, key)
		}
	}
	// add new group
	n.triggerGroups[utils.GenIDShort()] = newGroup

	return nil
}

func (n *neuron) AddCastGroup(groupName string, links ...core.Link) error {
	if groupName == "" {
		return fmt.Errorf("group name is empty")
	}
	for _, l := range links {
		if !n.hasOutLink(l.GetID()) {
			return errors.ErrOutLinkNotFound(l.GetID(), n.GetID())
		}
	}
	// init
	if n.castGroups == nil {
		n.castGroups = map[string]map[string]struct{}{
			processor.DefaultCastGroupName: map[string]struct{}{},
		}
	}
	if n.castGroups[processor.DefaultCastGroupName] == nil {
		n.castGroups[processor.DefaultCastGroupName] = map[string]struct{}{}
	}
	if n.castGroups[groupName] == nil {
		n.castGroups[groupName] = map[string]struct{}{}
	}

	for _, l := range links {
		_, ok := n.castGroups[processor.DefaultCastGroupName][l.GetID()]
		if ok {
			delete(n.castGroups[processor.DefaultCastGroupName], l.GetID())
		}
		// add link to group
		n.castGroups[groupName][l.GetID()] = struct{}{}
	}

	return nil
}

func (n *neuron) BindCastGroupSelectFunc(selectFn func(bcr processor.BrainContextReader) string) {
	n.bindCastGroupSelector(processor.NewFuncSelector(selectFn))
}

func (n *neuron) BindCastGroupSelector(selector processor.Selector) {
	n.bindCastGroupSelector(selector)
}

func (n *neuron) bindCastGroupSelector(selector processor.Selector) {
	n.selector = selector
}

func (n *neuron) addInLink(linkID string) {
	n.triggerGroups[utils.GenIDShort()] = []string{linkID}
}

func (n *neuron) addOutLink(linkID string) {
	if _, ok := n.castGroups[processor.DefaultCastGroupName]; !ok {
		n.castGroups[processor.DefaultCastGroupName] = make(map[string]struct{})
	}
	n.castGroups[processor.DefaultCastGroupName][linkID] = struct{}{}
}

func (n *neuron) hasInLink(linkID string) bool {
	for _, group := range n.triggerGroups {
		for _, l := range group {
			if l == linkID {
				return true
			}
		}
	}

	return false
}

func (n *neuron) hasOutLink(linkID string) bool {
	for _, group := range n.castGroups {
		for l, _ := range group {
			if l == linkID {
				return true
			}
		}
	}

	return false
}
