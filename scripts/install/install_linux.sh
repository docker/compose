#!/bin/sh

#   Copyright 2020 Docker Compose CLI authors

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

# Script to install the Docker Compose CLI on Ubuntu (Beta).

set -eu

RELEASE_URL=https://api.github.com/repos/docker/compose-cli/releases/latest
LINK_NAME="${LINK_NAME:-com.docker.cli}"
DRY_RUN="${DRY_RUN:-}"

desktop_install_url="https://www.docker.com/products/docker-desktop"
engine_install_url="https://docs.docker.com/get-docker/"

link_path="/usr/local/bin/${LINK_NAME}"
existing_cli_path="/usr/bin/docker"

manual_install() {
	echo "Please follow the manual install instructions"
}

is_new_cli() {
	cloud_version_str="$($1 version 2>/dev/null | grep 'Cloud integration' || true)"
	if [ -n "$cloud_version_str" ]; then
		echo 1
	else
        azure_version_str="$($1 version 2>/dev/null | grep 'Azure' || true)"
	    if [ -n "$azure_version_str" ]; then
		    echo 1
        fi
		echo 0
	fi
}

echo "Running checks..."

# Check OS
if [ "$(command -v uname)" ]; then
	case "$(uname -s)" in
		"Linux")
			# Check for Ubuntu/Debian based distro
			if ! [ -f "/etc/lsb-release" ]; then
				echo "Warning: This script has been tested on Ubuntu and may not work on other distributions"
			fi
			# Pass
			;;
		"Darwin")
			echo "Error: Script not needed on macOS, please install Docker Desktop Edge: $desktop_install_url"
			exit 1
			;;
		"*")
			echo "Error: Unsupported OS, please follow manual instructions"
			exit 1
			;;
	esac
else
	# Assume Windows
	echo "Error: Script not needed on Windows, please install Docker Desktop Edge: $desktop_install_url"
	exit 1
fi

user="$(id -un 2>/dev/null || true)"
sh_c='sh -c'
sudo_sh_c='sh -c'
if [ "$user" != 'root' ]; then
    if [ "$(command -v sudo)" ]; then
        sudo_sh_c='sudo -E sh -c'
    elif [ "$(command -v su)" ]; then
        sudo_sh_c='su -c'
    else
        echo "Error: This installer needs the ability to run commands as root."
        exit 1
    fi
fi

if [ -n "$DRY_RUN" ]; then
	sh_c='echo $sh_c'
	sudo_sh_c='echo $sudo_sh_c'
fi

# Check if Docker Engine is installed
if ! [ "$(command -v docker)" ]; then
	echo "Error: Docker Engine not found"
	echo "You need to install Docker first: $engine_install_url"
	exit 1
fi

download_cmd='curl -fsSLo'
# Check that system has curl installed
if ! [ "$(command -v curl)" ]; then
	echo "Error: curl not found"
	echo "Please install curl"
	exit 1
fi

if [ "$(uname -m)" = "aarch64" ]; then
	DOWNLOAD_URL=${DOWNLOAD_URL:-$(curl -s ${RELEASE_URL} | grep "browser_download_url.*docker-linux-arm64" | cut -d : -f 2,3)}
elif [ "$(uname -m)" = "s390x" ]; then
	DOWNLOAD_URL=${DOWNLOAD_URL:-$(curl -s ${RELEASE_URL} | grep "browser_download_url.*docker-linux-s390x" | cut -d : -f 2,3)}
else
	DOWNLOAD_URL=${DOWNLOAD_URL:-$(curl -s ${RELEASE_URL} | grep "browser_download_url.*docker-linux-amd64" | cut -d : -f 2,3)}
fi

# Check if the Compose CLI is already installed
if [ $(is_new_cli "docker") -eq 1 ]; then
	if [ $(is_new_cli "/usr/local/bin/docker") -eq 1 ]; then
		echo "You already have the Docker Compose CLI installed, overriding with latest version"
		download_dir=$($sh_c 'mktemp -d')
		$sh_c "${download_cmd} ${download_dir}/docker ${DOWNLOAD_URL}"
		$sudo_sh_c "install -m 775 ${download_dir}/docker /usr/local/bin/docker"
		exit 0
	fi
	echo "You already have the Docker Compose CLI installed, in a different location."
	exit 1
fi

# Check if this script has already been run
if [ -f "${link_path}" ]; then
	echo "Error: This script appears to have been run as ${link_path} exists"
	echo "Please uninstall and rerun this script or follow the manual instructions"
	exit 1
fi

# Check current Docker CLI is installed to /usr/bin/
if ! [ -f "${existing_cli_path}" ]; then
	echo "Error: This script only works if the Docker CLI is installed to /usr/bin/"
	manual_install
	exit 1
fi

# Check that PATH contains /usr/bin and /usr/local/bin and that the latter is
# higher priority
path_directories=$(echo "${PATH}" | tr ":" "\n")
usr_bin_pos=-1
usr_local_bin_pos=-1
count=0
for d in ${path_directories}; do
	if [ "${d}" = '/usr/bin' ]; then
		usr_bin_pos=$count
	fi
	if [ "${d}" = '/usr/local/bin' ]; then
		usr_local_bin_pos=$count
	fi
	count=$((count + 1))
done
if [ $usr_bin_pos -eq -1 ]; then
	echo "Error: /usr/bin not found in PATH"
	manual_install
	exit 1
elif [ $usr_local_bin_pos -eq -1 ]; then
	echo "Error: /usr/local/bin not found in PATH"
	manual_install
	exit 1
elif ! [ $usr_local_bin_pos -lt $usr_bin_pos ]; then
	echo "Error: /usr/local/bin is not ordered higher than /usr/bin in your PATH"
	manual_install
	exit 1
fi

echo "Checks passed!"
echo "Downloading CLI..."

# Download CLI to temporary directory
download_dir=$($sh_c 'mktemp -d')
$sh_c "${download_cmd} ${download_dir}/docker ${DOWNLOAD_URL}"

echo "Downloaded CLI!"
echo "Installing CLI..."

# Link existing Docker CLI
$sudo_sh_c "ln -s ${existing_cli_path} ${link_path}"

# Install downloaded CLI
$sudo_sh_c "install -m 775 ${download_dir}/docker /usr/local/bin/docker"

# Clear cache
cleared_cache=1
if [ "$(command hash)" ]; then
	$sh_c "hash -r"
elif [ "$(command rehash)" ]; then
	$sh_c "rehash"
else
	cleared_cache=
	echo "Warning: Unable to clear command cache"
fi

if [ -n "$DRY_RUN" ]; then
	exit 0
fi

if [ -n "$cleared_cache" ]; then
	# Check Compose CLI is working
	if [ $(is_new_cli "docker") -eq 0 ]; then
		echo "Error: Docker Compose CLI installation error"
		exit 1
	fi
	echo "Done!"
else
	echo "Please log out and in again to use the Docker Compose CLI"
fi
