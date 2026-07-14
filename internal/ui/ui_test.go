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

package ui

import "testing"

func TestWordmarkForWidth(t *testing.T) {
	if got := wordmarkForWidth(74); got != compactWordmark {
		t.Fatal("narrow terminal did not receive compact wordmark")
	}
	if got := wordmarkForWidth(120); got != wordmark {
		t.Fatal("wide terminal did not receive full wordmark")
	}
}
