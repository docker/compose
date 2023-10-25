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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSet_Has(t *testing.T) {
	x := NewSet[string]("value")
	require.True(t, x.Has("value"))
	require.False(t, x.Has("VALUE"))
}

func TestSet_Diff(t *testing.T) {
	a := NewSet[int](1, 2)
	b := NewSet[int](2, 3)
	require.ElementsMatch(t, []int{1}, a.Diff(b).Elements())
	require.ElementsMatch(t, []int{3}, b.Diff(a).Elements())
}

func TestSet_Union(t *testing.T) {
	a := NewSet[int](1, 2)
	b := NewSet[int](2, 3)
	require.ElementsMatch(t, []int{1, 2, 3}, a.Union(b).Elements())
	require.ElementsMatch(t, []int{1, 2, 3}, b.Union(a).Elements())
}
