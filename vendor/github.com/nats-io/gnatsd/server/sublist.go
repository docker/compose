// Copyright 2016 Apcera Inc. All rights reserved.

// Package sublist is a routing mechanism to handle subject distribution
// and provides a facility to match subjects from published messages to
// interested subscribers. Subscribers can have wildcard subjects to match
// multiple published subjects.
package server

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
)

// Common byte variables for wildcards and token separator.
const (
	pwc   = '*'
	fwc   = '>'
	tsep  = "."
	btsep = '.'
)

// Sublist related errors
var (
	ErrInvalidSubject = errors.New("sublist: Invalid Subject")
	ErrNotFound       = errors.New("sublist: No Matches Found")
)

// cacheMax is used to bound limit the frontend cache
const slCacheMax = 1024

// A result structure better optimized for queue subs.
type SublistResult struct {
	psubs []*subscription
	qsubs [][]*subscription // don't make this a map, too expensive to iterate
}

// A Sublist stores and efficiently retrieves subscriptions.
type Sublist struct {
	sync.RWMutex
	genid     uint64
	matches   uint64
	cacheHits uint64
	inserts   uint64
	removes   uint64
	cache     map[string]*SublistResult
	root      *level
	count     uint32
}

// A node contains subscriptions and a pointer to the next level.
type node struct {
	next  *level
	psubs []*subscription
	qsubs [][]*subscription
}

// A level represents a group of nodes and special pointers to
// wildcard nodes.
type level struct {
	nodes    map[string]*node
	pwc, fwc *node
}

// Create a new default node.
func newNode() *node {
	return &node{psubs: make([]*subscription, 0, 4)}
}

// Create a new default level. We use FNV1A as the hash
// algortihm for the tokens, which should be short.
func newLevel() *level {
	return &level{nodes: make(map[string]*node)}
}

// New will create a default sublist
func NewSublist() *Sublist {
	return &Sublist{root: newLevel(), cache: make(map[string]*SublistResult)}
}

// Insert adds a subscription into the sublist
func (s *Sublist) Insert(sub *subscription) error {
	// copy the subject since we hold this and this might be part of a large byte slice.
	subject := string(sub.subject)
	tsa := [32]string{}
	tokens := tsa[:0]
	start := 0
	for i := 0; i < len(subject); i++ {
		if subject[i] == btsep {
			tokens = append(tokens, subject[start:i])
			start = i + 1
		}
	}
	tokens = append(tokens, subject[start:])

	s.Lock()

	sfwc := false
	l := s.root
	var n *node

	for _, t := range tokens {
		if len(t) == 0 || sfwc {
			s.Unlock()
			return ErrInvalidSubject
		}

		switch t[0] {
		case pwc:
			n = l.pwc
		case fwc:
			n = l.fwc
			sfwc = true
		default:
			n = l.nodes[t]
		}
		if n == nil {
			n = newNode()
			switch t[0] {
			case pwc:
				l.pwc = n
			case fwc:
				l.fwc = n
			default:
				l.nodes[t] = n
			}
		}
		if n.next == nil {
			n.next = newLevel()
		}
		l = n.next
	}
	if sub.queue == nil {
		n.psubs = append(n.psubs, sub)
	} else {
		// This is a queue subscription
		if i := findQSliceForSub(sub, n.qsubs); i >= 0 {
			n.qsubs[i] = append(n.qsubs[i], sub)
		} else {
			n.qsubs = append(n.qsubs, []*subscription{sub})
		}
	}

	s.count++
	s.inserts++

	s.addToCache(subject, sub)
	atomic.AddUint64(&s.genid, 1)

	s.Unlock()
	return nil
}

// Deep copy
func copyResult(r *SublistResult) *SublistResult {
	nr := &SublistResult{}
	nr.psubs = append([]*subscription(nil), r.psubs...)
	for _, qr := range r.qsubs {
		nqr := append([]*subscription(nil), qr...)
		nr.qsubs = append(nr.qsubs, nqr)
	}
	return nr
}

// addToCache will add the new entry to existing cache
// entries if needed. Assumes write lock is held.
func (s *Sublist) addToCache(subject string, sub *subscription) {
	for k, r := range s.cache {
		if matchLiteral(k, subject) {
			// Copy since others may have a reference.
			nr := copyResult(r)
			if sub.queue == nil {
				nr.psubs = append(nr.psubs, sub)
			} else {
				if i := findQSliceForSub(sub, nr.qsubs); i >= 0 {
					nr.qsubs[i] = append(nr.qsubs[i], sub)
				} else {
					nr.qsubs = append(nr.qsubs, []*subscription{sub})
				}
			}
			s.cache[k] = nr
		}
	}
}

// removeFromCache will remove the sub from any active cache entries.
// Assumes write lock is held.
func (s *Sublist) removeFromCache(subject string, sub *subscription) {
	for k := range s.cache {
		if !matchLiteral(k, subject) {
			continue
		}
		// Since someone else may be referecing, can't modify the list
		// safely, just let it re-populate.
		delete(s.cache, k)
	}
}

// Match will match all entries to the literal subject.
// It will return a set of results for both normal and queue subscribers.
func (s *Sublist) Match(subject string) *SublistResult {
	s.RLock()
	atomic.AddUint64(&s.matches, 1)
	rc, ok := s.cache[subject]
	s.RUnlock()
	if ok {
		atomic.AddUint64(&s.cacheHits, 1)
		return rc
	}

	tsa := [32]string{}
	tokens := tsa[:0]
	start := 0
	for i := 0; i < len(subject); i++ {
		if subject[i] == btsep {
			tokens = append(tokens, subject[start:i])
			start = i + 1
		}
	}
	tokens = append(tokens, subject[start:])

	// FIXME(dlc) - Make shared pool between sublist and client readLoop?
	result := &SublistResult{}

	s.Lock()
	matchLevel(s.root, tokens, result)

	// Add to our cache
	s.cache[subject] = result
	// Bound the number of entries to sublistMaxCache
	if len(s.cache) > slCacheMax {
		for k := range s.cache {
			delete(s.cache, k)
			break
		}
	}
	s.Unlock()

	return result
}

// This will add in a node's results to the total results.
func addNodeToResults(n *node, results *SublistResult) {
	results.psubs = append(results.psubs, n.psubs...)
	for _, qr := range n.qsubs {
		if len(qr) == 0 {
			continue
		}
		// Need to find matching list in results
		if i := findQSliceForSub(qr[0], results.qsubs); i >= 0 {
			results.qsubs[i] = append(results.qsubs[i], qr...)
		} else {
			results.qsubs = append(results.qsubs, qr)
		}
	}
}

// We do not use a map here since we want iteration to be past when
// processing publishes in L1 on client. So we need to walk sequentially
// for now. Keep an eye on this in case we start getting large number of
// different queue subscribers for the same subject.
func findQSliceForSub(sub *subscription, qsl [][]*subscription) int {
	if sub.queue == nil {
		return -1
	}
	for i, qr := range qsl {
		if len(qr) > 0 && bytes.Equal(sub.queue, qr[0].queue) {
			return i
		}
	}
	return -1
}

// matchLevel is used to recursively descend into the trie.
func matchLevel(l *level, toks []string, results *SublistResult) {
	var pwc, n *node
	for i, t := range toks {
		if l == nil {
			return
		}
		if l.fwc != nil {
			addNodeToResults(l.fwc, results)
		}
		if pwc = l.pwc; pwc != nil {
			matchLevel(pwc.next, toks[i+1:], results)
		}
		n = l.nodes[t]
		if n != nil {
			l = n.next
		} else {
			l = nil
		}
	}
	if n != nil {
		addNodeToResults(n, results)
	}
	if pwc != nil {
		addNodeToResults(pwc, results)
	}
}

// lnt is used to track descent into levels for a removal for pruning.
type lnt struct {
	l *level
	n *node
	t string
}

// Remove will remove a subscription.
func (s *Sublist) Remove(sub *subscription) error {
	subject := string(sub.subject)
	tsa := [32]string{}
	tokens := tsa[:0]
	start := 0
	for i := 0; i < len(subject); i++ {
		if subject[i] == btsep {
			tokens = append(tokens, subject[start:i])
			start = i + 1
		}
	}
	tokens = append(tokens, subject[start:])

	s.Lock()
	defer s.Unlock()

	sfwc := false
	l := s.root
	var n *node

	// Track levels for pruning
	var lnts [32]lnt
	levels := lnts[:0]

	for _, t := range tokens {
		if len(t) == 0 || sfwc {
			return ErrInvalidSubject
		}
		if l == nil {
			return ErrNotFound
		}
		switch t[0] {
		case pwc:
			n = l.pwc
		case fwc:
			n = l.fwc
			sfwc = true
		default:
			n = l.nodes[t]
		}
		if n != nil {
			levels = append(levels, lnt{l, n, t})
			l = n.next
		} else {
			l = nil
		}
	}
	if !s.removeFromNode(n, sub) {
		return ErrNotFound
	}

	s.count--
	s.removes++

	for i := len(levels) - 1; i >= 0; i-- {
		l, n, t := levels[i].l, levels[i].n, levels[i].t
		if n.isEmpty() {
			l.pruneNode(n, t)
		}
	}
	s.removeFromCache(subject, sub)
	atomic.AddUint64(&s.genid, 1)

	return nil
}

// pruneNode is used to prune an empty node from the tree.
func (l *level) pruneNode(n *node, t string) {
	if n == nil {
		return
	}
	if n == l.fwc {
		l.fwc = nil
	} else if n == l.pwc {
		l.pwc = nil
	} else {
		delete(l.nodes, t)
	}
}

// isEmpty will test if the node has any entries. Used
// in pruning.
func (n *node) isEmpty() bool {
	if len(n.psubs) == 0 && len(n.qsubs) == 0 {
		if n.next == nil || n.next.numNodes() == 0 {
			return true
		}
	}
	return false
}

// Return the number of nodes for the given level.
func (l *level) numNodes() int {
	num := len(l.nodes)
	if l.pwc != nil {
		num++
	}
	if l.fwc != nil {
		num++
	}
	return num
}

// Removes a sub from a list.
func removeSubFromList(sub *subscription, sl []*subscription) ([]*subscription, bool) {
	for i := 0; i < len(sl); i++ {
		if sl[i] == sub {
			last := len(sl) - 1
			sl[i] = sl[last]
			sl[last] = nil
			sl = sl[:last]
			return shrinkAsNeeded(sl), true
		}
	}
	return sl, false
}

// Remove the sub for the given node.
func (s *Sublist) removeFromNode(n *node, sub *subscription) (found bool) {
	if n == nil {
		return false
	}
	if sub.queue == nil {
		n.psubs, found = removeSubFromList(sub, n.psubs)
		return found
	}

	// We have a queue group subscription here
	if i := findQSliceForSub(sub, n.qsubs); i >= 0 {
		n.qsubs[i], found = removeSubFromList(sub, n.qsubs[i])
		if len(n.qsubs[i]) == 0 {
			last := len(n.qsubs) - 1
			n.qsubs[i] = n.qsubs[last]
			n.qsubs[last] = nil
			n.qsubs = n.qsubs[:last]
			if len(n.qsubs) == 0 {
				n.qsubs = nil
			}
		}
		return found
	}
	return false
}

// Checks if we need to do a resize. This is for very large growth then
// subsequent return to a more normal size from unsubscribe.
func shrinkAsNeeded(sl []*subscription) []*subscription {
	lsl := len(sl)
	csl := cap(sl)
	// Don't bother if list not too big
	if csl <= 8 {
		return sl
	}
	pFree := float32(csl-lsl) / float32(csl)
	if pFree > 0.50 {
		return append([]*subscription(nil), sl...)
	}
	return sl
}

// Count returns the number of subscriptions.
func (s *Sublist) Count() uint32 {
	s.RLock()
	defer s.RUnlock()
	return s.count
}

// CacheCount returns the number of result sets in the cache.
func (s *Sublist) CacheCount() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.cache)
}

// Public stats for the sublist
type SublistStats struct {
	NumSubs      uint32  `json:"num_subscriptions"`
	NumCache     uint32  `json:"num_cache"`
	NumInserts   uint64  `json:"num_inserts"`
	NumRemoves   uint64  `json:"num_removes"`
	NumMatches   uint64  `json:"num_matches"`
	CacheHitRate float64 `json:"cache_hit_rate"`
	MaxFanout    uint32  `json:"max_fanout"`
	AvgFanout    float64 `json:"avg_fanout"`
}

// Stats will return a stats structure for the current state.
func (s *Sublist) Stats() *SublistStats {
	s.Lock()
	defer s.Unlock()

	st := &SublistStats{}
	st.NumSubs = s.count
	st.NumCache = uint32(len(s.cache))
	st.NumInserts = s.inserts
	st.NumRemoves = s.removes
	st.NumMatches = s.matches
	if s.matches > 0 {
		st.CacheHitRate = float64(s.cacheHits) / float64(s.matches)
	}
	// whip through cache for fanout stats
	tot, max := 0, 0
	for _, r := range s.cache {
		l := len(r.psubs) + len(r.qsubs)
		tot += l
		if l > max {
			max = l
		}
	}
	st.MaxFanout = uint32(max)
	if tot > 0 {
		st.AvgFanout = float64(tot) / float64(len(s.cache))
	}
	return st
}

// numLevels will return the maximum number of levels
// contained in the Sublist tree.
func (s *Sublist) numLevels() int {
	return visitLevel(s.root, 0)
}

// visitLevel is used to descend the Sublist tree structure
// recursively.
func visitLevel(l *level, depth int) int {
	if l == nil || l.numNodes() == 0 {
		return depth
	}

	depth++
	maxDepth := depth

	for _, n := range l.nodes {
		if n == nil {
			continue
		}
		newDepth := visitLevel(n.next, depth)
		if newDepth > maxDepth {
			maxDepth = newDepth
		}
	}
	if l.pwc != nil {
		pwcDepth := visitLevel(l.pwc.next, depth)
		if pwcDepth > maxDepth {
			maxDepth = pwcDepth
		}
	}
	if l.fwc != nil {
		fwcDepth := visitLevel(l.fwc.next, depth)
		if fwcDepth > maxDepth {
			maxDepth = fwcDepth
		}
	}
	return maxDepth
}

// IsValidSubject returns true if a subject is valid, false otherwise
func IsValidSubject(subject string) bool {
	if subject == "" {
		return false
	}
	sfwc := false
	tokens := strings.Split(string(subject), tsep)
	for _, t := range tokens {
		if len(t) == 0 || sfwc {
			return false
		}
		if len(t) > 1 {
			continue
		}
		switch t[0] {
		case fwc:
			sfwc = true
		}
	}
	return true
}

// IsValidLiteralSubject returns true if a subject is valid and literal (no wildcards), false otherwise
func IsValidLiteralSubject(subject string) bool {
	tokens := strings.Split(string(subject), tsep)
	for _, t := range tokens {
		if len(t) == 0 {
			return false
		}
		if len(t) > 1 {
			continue
		}
		switch t[0] {
		case pwc, fwc:
			return false
		}
	}
	return true
}

// matchLiteral is used to test literal subjects, those that do not have any
// wildcards, with a target subject. This is used in the cache layer.
func matchLiteral(literal, subject string) bool {
	li := 0
	ll := len(literal)
	for i := 0; i < len(subject); i++ {
		if li >= ll {
			return false
		}
		b := subject[i]
		switch b {
		case pwc:
			// Skip token in literal
			ll := len(literal)
			for {
				if li >= ll || literal[li] == btsep {
					li--
					break
				}
				li++
			}
		case fwc:
			return true
		default:
			if b != literal[li] {
				return false
			}
		}
		li++
	}
	// Make sure we have processed all of the literal's chars..
	if li < ll {
		return false
	}
	return true
}
