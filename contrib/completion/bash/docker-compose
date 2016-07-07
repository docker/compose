#!bash
#
# bash completion for docker-compose
#
# This work is based on the completion for the docker command.
#
# This script provides completion of:
#  - commands and their options
#  - service names
#  - filepaths
#
# To enable the completions either:
#  - place this file in /etc/bash_completion.d
#  or
#  - copy this file to e.g. ~/.docker-compose-completion.sh and add the line
#    below to your .bashrc after bash completion features are loaded
#    . ~/.docker-compose-completion.sh


__docker_compose_q() {
	docker-compose 2>/dev/null $daemon_options "$@"
}

# Transforms a multiline list of strings into a single line string
# with the words separated by "|".
__docker_compose_to_alternatives() {
	local parts=( $1 )
	local IFS='|'
	echo "${parts[*]}"
}

# Transforms a multiline list of options into an extglob pattern
# suitable for use in case statements.
__docker_compose_to_extglob() {
	local extglob=$( __docker_compose_to_alternatives "$1" )
	echo "@($extglob)"
}

# suppress trailing whitespace
__docker_compose_nospace() {
	# compopt is not available in ancient bash versions
	type compopt &>/dev/null && compopt -o nospace
}

# Extracts all service names from the compose file.
___docker_compose_all_services_in_compose_file() {
	__docker_compose_q config --services
}

# All services, even those without an existing container
__docker_compose_services_all() {
	COMPREPLY=( $(compgen -W "$(___docker_compose_all_services_in_compose_file)" -- "$cur") )
}

# All services that have an entry with the given key in their compose_file section
___docker_compose_services_with_key() {
	# flatten sections under "services" to one line, then filter lines containing the key and return section name
	__docker_compose_q config \
		| sed -n -e '/^services:/,/^[^ ]/p' \
		| sed -n 's/^  //p' \
		| awk '/^[a-zA-Z0-9]/{printf "\n"};{printf $0;next;}' \
		| awk -F: -v key=": +$1:" '$0 ~ key {print $1}'
}

# All services that are defined by a Dockerfile reference
__docker_compose_services_from_build() {
	COMPREPLY=( $(compgen -W "$(___docker_compose_services_with_key build)" -- "$cur") )
}

# All services that are defined by an image
__docker_compose_services_from_image() {
	COMPREPLY=( $(compgen -W "$(___docker_compose_services_with_key image)" -- "$cur") )
}

# The services for which containers have been created, optionally filtered
# by a boolean expression passed in as argument.
__docker_compose_services_with() {
	local containers names
	containers="$(__docker_compose_q ps -q)"
	names=$(docker 2>/dev/null inspect -f "{{if ${1:-true}}}{{range \$k, \$v := .Config.Labels}}{{if eq \$k \"com.docker.compose.service\"}}{{\$v}}{{end}}{{end}}{{end}}" $containers)
	COMPREPLY=( $(compgen -W "$names" -- "$cur") )
}

# The services for which at least one paused container exists
__docker_compose_services_paused() {
	__docker_compose_services_with '.State.Paused'
}

# The services for which at least one running container exists
__docker_compose_services_running() {
	__docker_compose_services_with '.State.Running'
}

# The services for which at least one stopped container exists
__docker_compose_services_stopped() {
	__docker_compose_services_with 'not .State.Running'
}


_docker_compose_build() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--force-rm --help --no-cache --pull" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_from_build
			;;
	esac
}


_docker_compose_bundle() {
	case "$prev" in
		--output|-o)
			_filedir
			return
			;;
	esac

	COMPREPLY=( $( compgen -W "--fetch-digests --help --output -o" -- "$cur" ) )
}


_docker_compose_config() {
	COMPREPLY=( $( compgen -W "--help --quiet -q --services" -- "$cur" ) )
}


_docker_compose_create() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--force-recreate --help --no-build --no-recreate" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_all
			;;
	esac
}


_docker_compose_docker_compose() {
	case "$prev" in
		--tlscacert|--tlscert|--tlskey)
			_filedir
			return
			;;
		--file|-f)
			_filedir "y?(a)ml"
			return
			;;
		$(__docker_compose_to_extglob "$daemon_options_with_args") )
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "$daemon_boolean_options  $daemon_options_with_args --help -h --verbose --version -v" -- "$cur" ) )
			;;
		*)
			COMPREPLY=( $( compgen -W "${commands[*]}" -- "$cur" ) )
			;;
	esac
}


_docker_compose_down() {
	case "$prev" in
		--rmi)
			COMPREPLY=( $( compgen -W "all local" -- "$cur" ) )
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --rmi --volumes -v --remove-orphans" -- "$cur" ) )
			;;
	esac
}


_docker_compose_events() {
	case "$prev" in
		--json)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --json" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_all
			;;
	esac
}


_docker_compose_exec() {
	case "$prev" in
		--index|--user)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "-d --help --index --privileged -T --user" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_running
			;;
	esac
}


_docker_compose_help() {
	COMPREPLY=( $( compgen -W "${commands[*]}" -- "$cur" ) )
}


_docker_compose_kill() {
	case "$prev" in
		-s)
			COMPREPLY=( $( compgen -W "SIGHUP SIGINT SIGKILL SIGUSR1 SIGUSR2" -- "$(echo $cur | tr '[:lower:]' '[:upper:]')" ) )
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help -s" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_running
			;;
	esac
}


_docker_compose_logs() {
	case "$prev" in
		--tail)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--follow -f --help --no-color --tail --timestamps -t" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_all
			;;
	esac
}


_docker_compose_pause() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_running
			;;
	esac
}


_docker_compose_port() {
	case "$prev" in
		--protocol)
			COMPREPLY=( $( compgen -W "tcp udp" -- "$cur" ) )
			return;
			;;
		--index)
			return;
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --index --protocol" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_all
			;;
	esac
}


_docker_compose_ps() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help -q" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_all
			;;
	esac
}


_docker_compose_pull() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --ignore-pull-failures" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_from_image
			;;
	esac
}


_docker_compose_push() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --ignore-push-failures" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_all
			;;
	esac
}


_docker_compose_restart() {
	case "$prev" in
		--timeout|-t)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --timeout -t" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_running
			;;
	esac
}


_docker_compose_rm() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--force -f --help -v" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_stopped
			;;
	esac
}


_docker_compose_run() {
	case "$prev" in
		-e)
			COMPREPLY=( $( compgen -e -- "$cur" ) )
			__docker_compose_nospace
			return
			;;
		--entrypoint|--name|--user|-u|--workdir|-w)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "-d --entrypoint -e --help --name --no-deps --publish -p --rm --service-ports -T --user -u --workdir -w" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_all
			;;
	esac
}


_docker_compose_scale() {
	case "$prev" in
		=)
			COMPREPLY=("$cur")
			return
			;;
		--timeout|-t)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --timeout -t" -- "$cur" ) )
			;;
		*)
			COMPREPLY=( $(compgen -S "=" -W "$(___docker_compose_all_services_in_compose_file)" -- "$cur") )
			__docker_compose_nospace
			;;
	esac
}


_docker_compose_start() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_stopped
			;;
	esac
}


_docker_compose_stop() {
	case "$prev" in
		--timeout|-t)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --timeout -t" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_running
			;;
	esac
}


_docker_compose_unpause() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_paused
			;;
	esac
}


_docker_compose_up() {
	case "$prev" in
		--timeout|-t)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--abort-on-container-exit --build -d --force-recreate --help --no-build --no-color --no-deps --no-recreate --timeout -t --remove-orphans" -- "$cur" ) )
			;;
		*)
			__docker_compose_services_all
			;;
	esac
}


_docker_compose_version() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--short" -- "$cur" ) )
			;;
	esac
}


_docker_compose() {
	local previous_extglob_setting=$(shopt -p extglob)
	shopt -s extglob

	local commands=(
		build
		bundle
		config
		create
		down
		events
		exec
		help
		kill
		logs
		pause
		port
		ps
		pull
		push
		restart
		rm
		run
		scale
		start
		stop
		unpause
		up
		version
	)

	# options for the docker daemon that have to be passed to secondary calls to
	# docker-compose executed by this script
	local daemon_boolean_options="
		--skip-hostname-check
		--tls
		--tlsverify
	"
	local daemon_options_with_args="
		--file -f
		--host -H
		--project-name -p
		--tlscacert
		--tlscert
		--tlskey
	"

	COMPREPLY=()
	local cur prev words cword
	_get_comp_words_by_ref -n : cur prev words cword

	# search subcommand and invoke its handler.
	# special treatment of some top-level options
	local command='docker_compose'
	local daemon_options=()
	local counter=1

	while [ $counter -lt $cword ]; do
		case "${words[$counter]}" in
			$(__docker_compose_to_extglob "$daemon_boolean_options") )
				local opt=${words[counter]}
				daemon_options+=($opt)
				;;
			$(__docker_compose_to_extglob "$daemon_options_with_args") )
				local opt=${words[counter]}
				local arg=${words[++counter]}
				daemon_options+=($opt $arg)
				;;
			-*)
				;;
			*)
				command="${words[$counter]}"
				break
				;;
		esac
		(( counter++ ))
	done

	local completions_func=_docker_compose_${command//-/_}
	declare -F $completions_func >/dev/null && $completions_func

	eval "$previous_extglob_setting"
	return 0
}

complete -F _docker_compose docker-compose
