//  Copyright (c) 2017 Couchbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package regexp

import "fmt"

// StateLimit is the maximum number of states allowed
const StateLimit = 10000

// ErrTooManyStates is returned if you attempt to build a Levenshtein
// automaton which requries too many states.
var ErrTooManyStates = fmt.Errorf("dfa contains more than %d states",
	StateLimit)

type dfaBuilder struct {
	dfa   *dfa
	cache map[string]uint
}

func newDfaBuilder(insts prog) *dfaBuilder {
	return &dfaBuilder{
		dfa: &dfa{
			insts:  insts,
			states: make([]*state, 0, 16),
		},
		cache: make(map[string]uint, 1024),
	}
}

func (d *dfaBuilder) build() (*dfa, error) {
	cur := newSparseSet(uint(len(d.dfa.insts)))
	next := newSparseSet(uint(len(d.dfa.insts)))

	d.dfa.add(cur, 0)
	states := uintpStack{d.cachedState(cur)}
	seen := make(map[uint]struct{})
	var s *uint
	states, s = states.Pop()
	for s != nil {
		for b := 0; b < 256; b++ {
			ns := d.runState(cur, next, *s, byte(b))
			if ns != nil {
				if _, ok := seen[*ns]; !ok {
					seen[*ns] = struct{}{}
					states = states.Push(ns)
				}
			}
			if len(d.dfa.states) > StateLimit {
				return nil, ErrTooManyStates
			}
		}
		states, s = states.Pop()
	}
	return d.dfa, nil
}

func (d *dfaBuilder) runState(cur, next *sparseSet, state uint, b byte) *uint {
	cur.Clear()
	for _, ip := range d.dfa.states[state].insts {
		cur.Add(ip)
	}
	d.dfa.run(cur, next, b)
	nextState := d.cachedState(next)
	d.dfa.states[state].next[b] = nextState
	return nextState
}

func (d *dfaBuilder) cachedState(set *sparseSet) *uint {
	var insts []uint
	var isMatch bool
	for i := uint(0); i < uint(set.Len()); i++ {
		ip := set.Get(i)
		switch d.dfa.insts[ip].op {
		case OpRange:
			insts = append(insts, ip)
		case OpMatch:
			isMatch = true
			insts = append(insts, ip)
		}
	}
	if len(insts) == 0 {
		return nil
	}
	k := fmt.Sprintf("%v", insts)
	v, ok := d.cache[k]
	if ok {
		return &v
	}
	d.dfa.states = append(d.dfa.states, &state{
		insts: insts,
		next:  make([]*uint, 256),
		match: isMatch,
	})
	newV := uint(len(d.dfa.states) - 1)
	d.cache[k] = newV
	return &newV
}

type dfa struct {
	insts  prog
	states []*state
}

func (d *dfa) add(set *sparseSet, ip uint) {
	if set.Contains(ip) {
		return
	}
	set.Add(ip)
	switch d.insts[ip].op {
	case OpJmp:
		d.add(set, d.insts[ip].to)
	case OpSplit:
		d.add(set, d.insts[ip].splitA)
		d.add(set, d.insts[ip].splitB)
	}
}

func (d *dfa) run(from, to *sparseSet, b byte) bool {
	to.Clear()
	var isMatch bool
	for i := uint(0); i < uint(from.Len()); i++ {
		ip := from.Get(i)
		switch d.insts[ip].op {
		case OpMatch:
			isMatch = true
		case OpRange:
			if d.insts[ip].rangeStart <= b &&
				b <= d.insts[ip].rangeEnd {
				d.add(to, ip+1)
			}
		}
	}
	return isMatch
}

type state struct {
	insts []uint
	next  []*uint
	match bool
}

type uintpStack []*uint

func (s uintpStack) Push(v *uint) uintpStack {
	return append(s, v)
}

func (s uintpStack) Pop() (uintpStack, *uint) {
	l := len(s)
	if l < 1 {
		return s, nil
	}
	return s[:l-1], s[l-1]
}
