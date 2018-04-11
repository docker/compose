#!/bin/bash
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

__docker_compose_previous_extglob_setting=$(shopt -p extglob)
shopt -s extglob

__docker_compose_q() {
	docker-compose 2>/dev/null "${top_level_options[@]}" "$@"
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

# Determines whether the option passed as the first argument exist on
# the commandline. The option may be a pattern, e.g. `--force|-f`.
__docker_compose_has_option() {
	local pattern="$1"
	for (( i=2; i < $cword; ++i)); do
		if [[ ${words[$i]} =~ ^($pattern)$ ]] ; then
			return 0
		fi
	done
	return 1
}

# Returns `key` if we are currently completing the value of a map option (`key=value`)
# which matches the extglob passed in as an argument.
# This function is needed for key-specific completions.
__docker_compose_map_key_of_current_option() {
        local glob="$1"

        local key glob_pos
        if [ "$cur" = "=" ] ; then        # key= case
                key="$prev"
                glob_pos=$((cword - 2))
        elif [[ $cur == *=* ]] ; then     # key=value case (OSX)
                key=${cur%=*}
                glob_pos=$((cword - 1))
        elif [ "$prev" = "=" ] ; then
                key=${words[$cword - 2]}  # key=value case
                glob_pos=$((cword - 3))
        else
                return
        fi

        [ "${words[$glob_pos]}" = "=" ] && ((glob_pos--))  # --option=key=value syntax

        [[ ${words[$glob_pos]} == @($glob) ]] && echo "$key"
}

# suppress trailing whitespace
__docker_compose_nospace() {
	# compopt is not available in ancient bash versions
	type compopt &>/dev/null && compopt -o nospace
}


# Outputs a list of all defined services, regardless of their running state.
# Arguments for `docker-compose ps` may be passed in order to filter the service list,
# e.g. `status=running`.
__docker_compose_services() {
	__docker_compose_q ps --services "$@"
}

# Applies completion of services based on the current value of `$cur`.
# Arguments for `docker-compose ps` may be passed in order to filter the service list,
# see `__docker_compose_services`.
__docker_compose_complete_services() {
	COMPREPLY=( $(compgen -W "$(__docker_compose_services "$@")" -- "$cur") )
}

# The services for which at least one running container exists
__docker_compose_complete_running_services() {
	local names=$(__docker_compose_complete_services --filter status=running)
	COMPREPLY=( $(compgen -W "$names" -- "$cur") )
}


_docker_compose_build() {
	case "$prev" in
		--build-arg)
			COMPREPLY=( $( compgen -e -- "$cur" ) )
			__docker_compose_nospace
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--build-arg --compress --force-rm --help --memory --no-cache --pull" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_services --filter source=build
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

	COMPREPLY=( $( compgen -W "--push-images --help --output -o" -- "$cur" ) )
}


_docker_compose_config() {
	COMPREPLY=( $( compgen -W "--help --quiet -q --resolve-image-digests --services --volumes" -- "$cur" ) )
}


_docker_compose_create() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--build --force-recreate --help --no-build --no-recreate" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_services
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
		--log-level)
			COMPREPLY=( $( compgen -W "debug info warning error critical" -- "$cur" ) )
			return
			;;
		--project-directory)
			_filedir -d
			return
			;;
		$(__docker_compose_to_extglob "$daemon_options_with_args") )
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "$daemon_boolean_options $daemon_options_with_args $top_level_options_with_args --help -h --no-ansi --verbose --version -v" -- "$cur" ) )
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
		--timeout|-t)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --rmi --timeout -t --volumes -v --remove-orphans" -- "$cur" ) )
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
			__docker_compose_complete_services
			;;
	esac
}


_docker_compose_exec() {
	case "$prev" in
		--index|--user|-u|--workdir|-w)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "-d --detach --help --index --privileged -T --user -u --workdir -w" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_running_services
			;;
	esac
}


_docker_compose_help() {
	COMPREPLY=( $( compgen -W "${commands[*]}" -- "$cur" ) )
}

_docker_compose_images() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --quiet -q" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_services
			;;
	esac
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
			__docker_compose_complete_running_services
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
			__docker_compose_complete_services
			;;
	esac
}


_docker_compose_pause() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_running_services
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
			__docker_compose_complete_services
			;;
	esac
}


_docker_compose_ps() {
	local key=$(__docker_compose_map_key_of_current_option '--filter')
	case "$key" in
		source)
			COMPREPLY=( $( compgen -W "build image" -- "${cur##*=}" ) )
			return
			;;
		status)
			COMPREPLY=( $( compgen -W "paused restarting running stopped" -- "${cur##*=}" ) )
			return
			;;
	esac

	case "$prev" in
		--filter)
			COMPREPLY=( $( compgen -W "source status" -S "=" -- "$cur" ) )
			__docker_compose_nospace
			return;
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --quiet -q --services --filter" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_services
			;;
	esac
}


_docker_compose_pull() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --ignore-pull-failures --include-deps --no-parallel --quiet -q" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_services --filter source=image
			;;
	esac
}


_docker_compose_push() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help --ignore-push-failures" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_services
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
			__docker_compose_complete_running_services
			;;
	esac
}


_docker_compose_rm() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--force -f --help --stop -s -v" -- "$cur" ) )
			;;
		*)
			if __docker_compose_has_option "--stop|-s" ; then
				__docker_compose_complete_services
			else
				__docker_compose_complete_services --filter status=stopped
			fi
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
		--entrypoint|--label|-l|--name|--user|-u|--volume|-v|--workdir|-w)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--detach -d --entrypoint -e --help --label -l --name --no-deps --publish -p --rm --service-ports -T --use-aliases --user -u --volume -v --workdir -w" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_services
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
			COMPREPLY=( $(compgen -S "=" -W "$(__docker_compose_services)" -- "$cur") )
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
			__docker_compose_complete_services --filter status=stopped
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
			__docker_compose_complete_running_services
			;;
	esac
}


_docker_compose_top() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_running_services
			;;
	esac
}


_docker_compose_unpause() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_services --filter status=paused
			;;
	esac
}


_docker_compose_up() {
	case "$prev" in
		=)
			COMPREPLY=("$cur")
			return
			;;
		--exit-code-from)
			__docker_compose_complete_services
			return
			;;
		--scale)
			COMPREPLY=( $(compgen -S "=" -W "$(__docker_compose_services)" -- "$cur") )
			__docker_compose_nospace
			return
			;;
		--timeout|-t)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--abort-on-container-exit --always-recreate-deps --build -d --detach --exit-code-from --force-recreate --help --no-build --no-color --no-deps --no-recreate --no-start --renew-anon-volumes -V --remove-orphans --scale --timeout -t" -- "$cur" ) )
			;;
		*)
			__docker_compose_complete_services
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
		images
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
		top
		unpause
		up
		version
	)

	# Options for the docker daemon that have to be passed to secondary calls to
	# docker-compose executed by this script.
	local daemon_boolean_options="
		--skip-hostname-check
		--tls
		--tlsverify
	"
	local daemon_options_with_args="
		--file -f
		--host -H
		--project-directory
		--project-name -p
		--tlscacert
		--tlscert
		--tlskey
	"

	# These options are require special treatment when searching the command.
	local top_level_options_with_args="
		--log-level
	"

	COMPREPLY=()
	local cur prev words cword
	_get_comp_words_by_ref -n : cur prev words cword

	# search subcommand and invoke its handler.
	# special treatment of some top-level options
	local command='docker_compose'
	local top_level_options=()
	local counter=1

	while [ $counter -lt $cword ]; do
		case "${words[$counter]}" in
			$(__docker_compose_to_extglob "$daemon_boolean_options") )
				local opt=${words[counter]}
				top_level_options+=($opt)
				;;
			$(__docker_compose_to_extglob "$daemon_options_with_args") )
				local opt=${words[counter]}
				local arg=${words[++counter]}
				top_level_options+=($opt $arg)
				;;
			$(__docker_compose_to_extglob "$top_level_options_with_args") )
				(( counter++ ))
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

eval "$__docker_compose_previous_extglob_setting"
unset __docker_compose_previous_extglob_setting

complete -F _docker_compose docker-compose docker-compose.exe
