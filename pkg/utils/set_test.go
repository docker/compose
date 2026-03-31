/*
   Copyright 2022 Docker Compose CLI authors

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

import (
	"slices"
	"testing"

	"gotest.tools/v3/assert"
)

func TestSet_Has(t *testing.T) {
	x := NewSet[string]("value")
	assert.Check(t, x.Has("value"))
	assert.Check(t, !x.Has("VALUE"))
}

func TestSet_Diff(t *testing.T) {
	a := NewSet[int](1, 2)
	b := NewSet[int](2, 3)
	assert.DeepEqual(t, []int{1}, a.Diff(b).Elements())
	assert.DeepEqual(t, []int{3}, b.Diff(a).Elements())
}

func TestSet_Union(t *testing.T) {
	a := NewSet[int](1, 2)
	b := NewSet[int](2, 3)

	actual := a.Union(b).Elements()
	slices.Sort(actual)
	assert.DeepEqual(t, []int{1, 2, 3}, actual)

	actual = b.Union(a).Elements()
	slices.Sort(actual)
	assert.DeepEqual(t, []int{1, 2, 3}, actual)
}
