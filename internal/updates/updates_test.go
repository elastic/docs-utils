// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Apache License,
// Version 2.0 (the "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package updates

import "testing"

func TestSemverGT(t *testing.T) {
	for _, test := range []struct {
		a, b string
		want bool
	}{
		{"2.0.0", "1.9.9", true},
		{"1.2.1", "1.2.0", true},
		{"1.2.0", "1.2.0", false},
		{"1.2.0", "1.3.0", false},
	} {
		if got := semverGT(test.a, test.b); got != test.want {
			t.Errorf("semverGT(%q, %q) = %t, want %t", test.a, test.b, got, test.want)
		}
	}
}
