# NAME
   runc list - lists containers started by runc with the given root

# SYNOPSIS
   runc list [command options] [arguments...]

# DESCRIPTION
   The default format is table.  The following will output the list of containers
in json format:

    # runc list -f json

# OPTIONS
   --format, -f         select one of: table or json.
