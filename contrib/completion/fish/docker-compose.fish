# Tab completion for docker-compose (https://github.com/docker/compose).
# Version: 1.9.0

complete -e -c docker-compose

for line in (docker-compose --help | \
             string match -r '^\s+\w+\s+[^\n]+' | \
             string trim)
  set -l doc (string split -m 1 ' ' -- $line)
  complete -c docker-compose -n '__fish_use_subcommand' -xa $doc[1] --description $doc[2]
end

complete -c docker-compose -s f -l file -r                -d 'Specify an alternate compose file'
complete -c docker-compose -s p -l project-name -x        -d 'Specify an alternate project name'
complete -c docker-compose -l env-file -r                 -d 'Specify an alternate environment file (default: .env)'
complete -c docker-compose -l verbose                     -d 'Show more output'
complete -c docker-compose -s H -l host -x                -d 'Daemon socket to connect to'
complete -c docker-compose -l tls                         -d 'Use TLS; implied by --tlsverify'
complete -c docker-compose -l tlscacert -r                -d 'Trust certs signed only by this CA'
complete -c docker-compose -l tlscert -r                  -d 'Path to TLS certificate file'
complete -c docker-compose -l tlskey -r                   -d 'Path to TLS key file'
complete -c docker-compose -l tlsverify                   -d 'Use TLS and verify the remote'
complete -c docker-compose -l skip-hostname-check         -d "Don't check the daemon's hostname against the name specified in the client certificate (for example if your docker host is an IP address)"
complete -c docker-compose -s h -l help                   -d 'Print usage'
complete -c docker-compose -s v -l version                -d 'Print version and exit'
