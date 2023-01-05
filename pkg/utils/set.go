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

func (s Set[T]) Add(t T) {
	s[t] = struct{}{}
}

func (s Set[T]) Remove(t T) {
	delete(s, t)
}

func (s Set[T]) Values() []T {
	v := make([]T, 0, len(s))
	for k := range s {
		v = append(v, k)
	}
	return v
}
