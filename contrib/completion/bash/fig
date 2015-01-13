#!bash
#
# bash completion for fig commands
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
#  - copy this file and add the line below to your .bashrc after
#    bash completion features are loaded
#     . docker.bash
#
# Note:
# Some completions require the current user to have sufficient permissions
# to execute the docker command.


# Extracts all service names from the figfile.
___fig_all_services_in_figfile() {
	awk -F: '/^[a-zA-Z0-9]/{print $1}' "${fig_file:-fig.yml}"
}

# All services, even those without an existing container
__fig_services_all() {
	COMPREPLY=( $(compgen -W "$(___fig_all_services_in_figfile)" -- "$cur") )
}

# All services that have an entry with the given key in their figfile section
___fig_services_with_key() {
	# flatten sections to one line, then filter lines containing the key and return section name.
	awk '/^[a-zA-Z0-9]/{printf "\n"};{printf $0;next;}' fig.yml | awk -F: -v key=": +$1:" '$0 ~ key {print $1}'
}

# All services that are defined by a Dockerfile reference
__fig_services_from_build() {
	COMPREPLY=( $(compgen -W "$(___fig_services_with_key build)" -- "$cur") )
}

# All services that are defined by an image
__fig_services_from_image() {
	COMPREPLY=( $(compgen -W "$(___fig_services_with_key image)" -- "$cur") )
}

# The services for which containers have been created, optionally filtered
# by a boolean expression passed in as argument.
__fig_services_with() {
	local containers names
	containers="$(fig 2>/dev/null ${fig_file:+-f $fig_file} ${fig_project:+-p $fig_project} ps -q)"
	names=( $(docker 2>/dev/null inspect --format "{{if ${1:-true}}} {{ .Name }} {{end}}" $containers) )
	names=( ${names[@]%_*} )  # strip trailing numbers
	names=( ${names[@]#*_} )  # strip project name
	COMPREPLY=( $(compgen -W "${names[*]}" -- "$cur") )
}

# The services for which at least one running container exists
__fig_services_running() {
	__fig_services_with '.State.Running'
}

# The services for which at least one stopped container exists
__fig_services_stopped() {
	__fig_services_with 'not .State.Running'
}


_fig_build() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--no-cache" -- "$cur" ) )
			;;
		*)
			__fig_services_from_build
			;;
	esac
}


_fig_fig() {
	case "$prev" in
		--file|-f)
			_filedir
			return
			;;
		--project-name|-p)
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--help -h --verbose --version --file -f --project-name -p" -- "$cur" ) )
			;;
		*)
			COMPREPLY=( $( compgen -W "${commands[*]}" -- "$cur" ) )
			;;
	esac
}


_fig_help() {
	COMPREPLY=( $( compgen -W "${commands[*]}" -- "$cur" ) )
}


_fig_kill() {
	case "$prev" in
		-s)
			COMPREPLY=( $( compgen -W "SIGHUP SIGINT SIGKILL SIGUSR1 SIGUSR2" -- "$(echo $cur | tr '[:lower:]' '[:upper:]')" ) )
			return
			;;
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "-s" -- "$cur" ) )
			;;
		*)
			__fig_services_running
			;;
	esac
}


_fig_logs() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--no-color" -- "$cur" ) )
			;;
		*)
			__fig_services_all
			;;
	esac
}


_fig_port() {
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
			COMPREPLY=( $( compgen -W "--protocol --index" -- "$cur" ) )
			;;
		*)
			__fig_services_all
			;;
	esac
}


_fig_ps() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "-q" -- "$cur" ) )
			;;
		*)
			__fig_services_all
			;;
	esac
}


_fig_pull() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--allow-insecure-ssl" -- "$cur" ) )
			;;
		*)
			__fig_services_from_image
			;;
	esac
}


_fig_restart() {
	__fig_services_running
}


_fig_rm() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--force -v" -- "$cur" ) )
			;;
		*)
			__fig_services_stopped
			;;
	esac
}


_fig_run() {
	case "$prev" in
		-e)
			COMPREPLY=( $( compgen -e -- "$cur" ) )
			compopt -o nospace
			return
			;;
		--entrypoint)
			return
			;;	
	esac

	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--allow-insecure-ssl -d --entrypoint -e --no-deps --rm -T" -- "$cur" ) )
			;;
		*)
			__fig_services_all
			;;
	esac
}


_fig_scale() {
	case "$prev" in
		=)
			COMPREPLY=("$cur")
			;;
		*)
			COMPREPLY=( $(compgen -S "=" -W "$(___fig_all_services_in_figfile)" -- "$cur") )
			compopt -o nospace
			;;
	esac
}


_fig_start() {
	__fig_services_stopped
}


_fig_stop() {
	__fig_services_running
}


_fig_up() {
	case "$cur" in
		-*)
			COMPREPLY=( $( compgen -W "--allow-insecure-ssl -d --no-build --no-color --no-deps --no-recreate" -- "$cur" ) )
			;;
		*)
			__fig_services_all
			;;
	esac
}


_fig() {
	local commands=(
		build
		help
		kill
		logs
		port
		ps
		pull
		restart
		rm
		run
		scale
		start
		stop
		up
	)

	COMPREPLY=()
	local cur prev words cword
	_get_comp_words_by_ref -n : cur prev words cword

	# search subcommand and invoke its handler.
	# special treatment of some top-level options
	local command='fig'
	local counter=1
	local fig_file fig_project
	while [ $counter -lt $cword ]; do
		case "${words[$counter]}" in
			-f|--file)
				(( counter++ ))
				fig_file="${words[$counter]}"
				;;
			-p|--project-name)
				(( counter++ ))
				fig_project="${words[$counter]}"
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

	local completions_func=_fig_${command}
	declare -F $completions_func >/dev/null && $completions_func

	return 0
}

complete -F _fig fig
