/*
   Copyright 2024 Docker Compose CLI authors

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

package compose

import (
	"bytes"
	"strings"
	"testing"

	"github.com/docker/compose/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var topTestCases = []struct {
	name   string
	titles []string
	procs  [][]string

	header  topHeader
	entries []topEntries
	output  string
}{
	{
		name:    "noprocs",
		titles:  []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"},
		procs:   [][]string{},
		header:  topHeader{"SERVICE": 0, "#": 1},
		entries: []topEntries{},
		output:  "",
	},
	{
		name:   "simple",
		titles: []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"},
		procs:  [][]string{{"root", "1", "1", "0", "12:00", "?", "00:00:01", "/entrypoint"}},
		header: topHeader{
			"SERVICE": 0,
			"#":       1,
			"UID":     2,
			"PID":     3,
			"PPID":    4,
			"C":       5,
			"STIME":   6,
			"TTY":     7,
			"TIME":    8,
			"CMD":     9,
		},
		entries: []topEntries{
			{
				"SERVICE": "simple",
				"#":       "1",
				"UID":     "root",
				"PID":     "1",
				"PPID":    "1",
				"C":       "0",
				"STIME":   "12:00",
				"TTY":     "?",
				"TIME":    "00:00:01",
				"CMD":     "/entrypoint",
			},
		},
		output: trim(`
			SERVICE  #   UID   PID  PPID  C   STIME  TTY  TIME      CMD
			simple   1   root  1    1     0   12:00  ?    00:00:01  /entrypoint
		`),
	},
	{
		name:   "noppid",
		titles: []string{"UID", "PID", "C", "STIME", "TTY", "TIME", "CMD"},
		procs:  [][]string{{"root", "1", "0", "12:00", "?", "00:00:02", "/entrypoint"}},
		header: topHeader{
			"SERVICE": 0,
			"#":       1,
			"UID":     2,
			"PID":     3,
			"C":       4,
			"STIME":   5,
			"TTY":     6,
			"TIME":    7,
			"CMD":     8,
		},
		entries: []topEntries{
			{
				"SERVICE": "noppid",
				"#":       "1",
				"UID":     "root",
				"PID":     "1",
				"C":       "0",
				"STIME":   "12:00",
				"TTY":     "?",
				"TIME":    "00:00:02",
				"CMD":     "/entrypoint",
			},
		},
		output: trim(`
			SERVICE  #   UID   PID  C   STIME  TTY  TIME      CMD
			noppid   1   root  1    0   12:00  ?    00:00:02  /entrypoint
		`),
	},
	{
		name:   "extra-hdr",
		titles: []string{"UID", "GID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"},
		procs:  [][]string{{"root", "1", "1", "1", "0", "12:00", "?", "00:00:03", "/entrypoint"}},
		header: topHeader{
			"SERVICE": 0,
			"#":       1,
			"UID":     2,
			"GID":     3,
			"PID":     4,
			"PPID":    5,
			"C":       6,
			"STIME":   7,
			"TTY":     8,
			"TIME":    9,
			"CMD":     10,
		},
		entries: []topEntries{
			{
				"SERVICE": "extra-hdr",
				"#":       "1",
				"UID":     "root",
				"GID":     "1",
				"PID":     "1",
				"PPID":    "1",
				"C":       "0",
				"STIME":   "12:00",
				"TTY":     "?",
				"TIME":    "00:00:03",
				"CMD":     "/entrypoint",
			},
		},
		output: trim(`
			SERVICE    #   UID   GID  PID  PPID  C   STIME  TTY  TIME      CMD
			extra-hdr  1   root  1    1    1     0   12:00  ?    00:00:03  /entrypoint
		`),
	},
	{
		name:   "multiple",
		titles: []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"},
		procs: [][]string{
			{"root", "1", "1", "0", "12:00", "?", "00:00:04", "/entrypoint"},
			{"root", "123", "1", "0", "12:00", "?", "00:00:42", "sleep infinity"},
		},
		header: topHeader{
			"SERVICE": 0,
			"#":       1,
			"UID":     2,
			"PID":     3,
			"PPID":    4,
			"C":       5,
			"STIME":   6,
			"TTY":     7,
			"TIME":    8,
			"CMD":     9,
		},
		entries: []topEntries{
			{
				"SERVICE": "multiple",
				"#":       "1",
				"UID":     "root",
				"PID":     "1",
				"PPID":    "1",
				"C":       "0",
				"STIME":   "12:00",
				"TTY":     "?",
				"TIME":    "00:00:04",
				"CMD":     "/entrypoint",
			},
			{
				"SERVICE": "multiple",
				"#":       "1",
				"UID":     "root",
				"PID":     "123",
				"PPID":    "1",
				"C":       "0",
				"STIME":   "12:00",
				"TTY":     "?",
				"TIME":    "00:00:42",
				"CMD":     "sleep infinity",
			},
		},
		output: trim(`
			SERVICE   #   UID   PID  PPID  C   STIME  TTY  TIME      CMD
			multiple  1   root  1    1     0   12:00  ?    00:00:04  /entrypoint
			multiple  1   root  123  1     0   12:00  ?    00:00:42  sleep infinity
		`),
	},
}

// TestRunTopCore only tests the core functionality of runTop: formatting
// and printing of the output of (api.Service).Top().
func TestRunTopCore(t *testing.T) {
	t.Parallel()

	all := []api.ContainerProcSummary{}

	for _, tc := range topTestCases {
		summary := api.ContainerProcSummary{
			Name:      "not used",
			Titles:    tc.titles,
			Processes: tc.procs,
			Service:   tc.name,
			Replica:   "1",
		}
		all = append(all, summary)

		t.Run(tc.name, func(t *testing.T) {
			header, entries := collectTop([]api.ContainerProcSummary{summary})
			assert.Equal(t, tc.header, header)
			assert.Equal(t, tc.entries, entries)

			var buf bytes.Buffer
			err := topPrint(&buf, header, entries)

			require.NoError(t, err)
			assert.Equal(t, tc.output, buf.String())
		})
	}

	t.Run("all", func(t *testing.T) {
		header, entries := collectTop(all)
		assert.Equal(t, topHeader{
			"SERVICE": 0,
			"#":       1,
			"UID":     2,
			"PID":     3,
			"PPID":    4,
			"C":       5,
			"STIME":   6,
			"TTY":     7,
			"TIME":    8,
			"GID":     9,
			"CMD":     10,
		}, header)
		assert.Equal(t, []topEntries{
			{
				"SERVICE": "simple",
				"#":       "1",
				"UID":     "root",
				"PID":     "1",
				"PPID":    "1",
				"C":       "0",
				"STIME":   "12:00",
				"TTY":     "?",
				"TIME":    "00:00:01",
				"CMD":     "/entrypoint",
			}, {
				"SERVICE": "noppid",
				"#":       "1",
				"UID":     "root",
				"PID":     "1",
				"C":       "0",
				"STIME":   "12:00",
				"TTY":     "?",
				"TIME":    "00:00:02",
				"CMD":     "/entrypoint",
			}, {
				"SERVICE": "extra-hdr",
				"#":       "1",
				"UID":     "root",
				"GID":     "1",
				"PID":     "1",
				"PPID":    "1",
				"C":       "0",
				"STIME":   "12:00",
				"TTY":     "?",
				"TIME":    "00:00:03",
				"CMD":     "/entrypoint",
			}, {
				"SERVICE": "multiple",
				"#":       "1",
				"UID":     "root",
				"PID":     "1",
				"PPID":    "1",
				"C":       "0",
				"STIME":   "12:00",
				"TTY":     "?",
				"TIME":    "00:00:04",
				"CMD":     "/entrypoint",
			}, {
				"SERVICE": "multiple",
				"#":       "1",
				"UID":     "root",
				"PID":     "123",
				"PPID":    "1",
				"C":       "0",
				"STIME":   "12:00",
				"TTY":     "?",
				"TIME":    "00:00:42",
				"CMD":     "sleep infinity",
			},
		}, entries)

		var buf bytes.Buffer
		err := topPrint(&buf, header, entries)
		require.NoError(t, err)
		assert.Equal(t, trim(`
			SERVICE    #   UID   PID  PPID  C   STIME  TTY  TIME      GID  CMD
			simple     1   root  1    1     0   12:00  ?    00:00:01  -    /entrypoint
			noppid     1   root  1    -     0   12:00  ?    00:00:02  -    /entrypoint
			extra-hdr  1   root  1    1     0   12:00  ?    00:00:03  1    /entrypoint
			multiple   1   root  1    1     0   12:00  ?    00:00:04  -    /entrypoint
			multiple   1   root  123  1     0   12:00  ?    00:00:42  -    sleep infinity
		`), buf.String())
	})
}

func trim(s string) string {
	var out bytes.Buffer
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		out.WriteString(strings.TrimSpace(line))
		out.WriteRune('\n')
	}
	return out.String()
}
