/*
   Copyright 2020 Docker, Inc.

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

package mobyflags

import (
	"log"

	flag "github.com/spf13/pflag"
)

// AddMobyFlagsForRetrocompatibility adds retrocompatibility flags to our commands
func AddMobyFlagsForRetrocompatibility(flags *flag.FlagSet) {
	const logLevelFlag = "log-level"
	flags.StringP(logLevelFlag, "l", "info", `Set the logging level ("debug"|"info"|"warn"|"error"|"fatal")`)
	markHidden(flags, logLevelFlag)
}

func markHidden(flags *flag.FlagSet, flagName string) {
	err := flags.MarkHidden(flagName)
	if err != nil {
		log.Fatal(err)
	}
}
