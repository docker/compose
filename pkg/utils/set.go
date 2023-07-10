/*

   Copyright 2020 Docker Compose CLI authors
   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at
       http://www.apache.org/licenses/LICENSE-2.0
   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package utils

type Set[T comparable] map[T]struct{}

func (s Set[T]) Add(v T) {
	s[v] = struct{}{}
}

func (s Set[T]) AddAll(v ...T) {
	for _, e := range v {
		s[e] = struct{}{}
	}
}

func (s Set[T]) Remove(v T) bool {
	_, ok := s[v]
	if ok {
		delete(s, v)
	}
	return ok
}

func (s Set[T]) Clear() {
	for v := range s {
		delete(s, v)
	}
}

func (s Set[T]) Elements() []T {
	elements := make([]T, 0, len(s))
	for v := range s {
		elements = append(elements, v)
	}
	return elements
}

func (s Set[T]) RemoveAll(elements ...T) {
	for _, e := range elements {
		s.Remove(e)
	}
}
