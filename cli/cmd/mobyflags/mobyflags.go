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
