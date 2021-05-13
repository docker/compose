#!groovy

def dockerVersions = ['19.03.13', '18.09.9']
def baseImages = ['alpine', 'debian']
def pythonVersions = ['py37']

pipeline {
    agent none

    options {
        skipDefaultCheckout(true)
        buildDiscarder(logRotator(daysToKeepStr: '30'))
        timeout(time: 2, unit: 'HOURS')
        timestamps()
    }
    environment {
        DOCKER_BUILDKIT="1"
    }

    stages {
        stage('Build test images') {
            // TODO use declarative 1.5.0 `matrix` once available on CI
            parallel {
                stage('alpine') {
                    agent {
                        label 'linux && docker && ubuntu-2004 && amd64 && cgroup1'
                    }
                    steps {
                        buildImage('alpine')
                    }
                }
                stage('debian') {
                    agent {
                        label 'linux && docker && ubuntu-2004 && amd64 && cgroup1'
                    }
                    steps {
                        buildImage('debian')
                    }
                }
            }
        }
        stage('Test') {
            agent {
                label 'linux && docker && ubuntu-2004 && amd64 && cgroup1'
            }
            steps {
                // TODO use declarative 1.5.0 `matrix` once available on CI
                script {
                    def testMatrix = [:]
                    baseImages.each { baseImage ->
                      dockerVersions.each { dockerVersion ->
                        pythonVersions.each { pythonVersion ->
                          testMatrix["${baseImage}_${dockerVersion}_${pythonVersion}"] = runTests(dockerVersion, pythonVersion, baseImage)
                        }
                      }
                    }

                    parallel testMatrix
                }
            }
        }
        stage('Generate Changelog') {
            agent {
                label 'linux && docker && ubuntu-2004 && amd64 && cgroup1'
            }
            steps {
                checkout scm
                withCredentials([string(credentialsId: 'github-compose-release-test-token', variable: 'GITHUB_TOKEN')]) {
                    sh "./script/release/generate_changelog.sh"
                }
                archiveArtifacts artifacts: 'CHANGELOG.md'
                stash( name: "changelog", includes: 'CHANGELOG.md' )
            }
        }
        stage('Package') {
            parallel {
                stage('macosx binary') {
                    agent {
                        label 'mac-python'
                    }
                    environment {
                        DEPLOYMENT_TARGET="10.11"
                    }
                    steps {
                        checkout scm
                        sh './script/setup/osx'
                        sh 'tox -e py39 -- tests/unit'
                        sh './script/build/osx'
                        dir ('dist') {
                          checksum('docker-compose-Darwin-x86_64')
                          checksum('docker-compose-Darwin-x86_64.tgz')
                        }
                        archiveArtifacts artifacts: 'dist/*', fingerprint: true
                        dir("dist") {
                            stash name: "bin-darwin"
                        }
                    }
                }
                stage('linux binary') {
                    agent {
                        label 'linux && docker && ubuntu-2004 && amd64 && cgroup1'
                    }
                    steps {
                        checkout scm
                        sh ' ./script/build/linux'
                        dir ('dist') {
                          checksum('docker-compose-Linux-x86_64')
                        }
                        archiveArtifacts artifacts: 'dist/*', fingerprint: true
                        dir("dist") {
                            stash name: "bin-linux"
                        }
                    }
                }
                stage('windows binary') {
                    agent {
                        label 'windows-python'
                    }
                    environment {
                        PATH = "C:\\Python39;C:\\Python39\\Scripts;$PATH"
                    }
                    steps {
                        checkout scm
                        bat 'tox.exe -e py39 -- tests/unit'
                        powershell '.\\script\\build\\windows.ps1'
                        dir ('dist') {
                            checksum('docker-compose-Windows-x86_64.exe')
                        }
                        archiveArtifacts artifacts: 'dist/*', fingerprint: true
                        dir("dist") {
                            stash name: "bin-win"
                        }
                    }
                }
                stage('alpine image') {
                    agent {
                        label 'linux && docker && ubuntu-2004 && amd64 && cgroup1'
                    }
                    steps {
                        buildRuntimeImage('alpine')
                    }
                }
                stage('debian image') {
                    agent {
                        label 'linux && docker && ubuntu-2004 && amd64 && cgroup1'
                    }
                    steps {
                        buildRuntimeImage('debian')
                    }
                }
            }
        }
        stage('Release') {
            when {
                buildingTag()
            }
            parallel {
                stage('Pushing images') {
                    agent {
                        label 'linux && docker && ubuntu-2004 && amd64 && cgroup1'
                    }
                    steps {
                        pushRuntimeImage('alpine')
                        pushRuntimeImage('debian')
                    }
                }
                stage('Creating Github Release') {
                    agent {
                        label 'linux && docker && ubuntu-2004 && amd64 && cgroup1'
                    }
                    environment {
                        GITHUB_TOKEN = credentials('github-release-token')
                    }
                    steps {
                        checkout scm
                        sh 'mkdir -p dist'
                        dir("dist") {
                            unstash "bin-darwin"
                            unstash "bin-linux"
                            unstash "bin-win"
                            unstash "changelog"
                            sh("""
                                curl -SfL https://github.com/github/hub/releases/download/v2.13.0/hub-linux-amd64-2.13.0.tgz | tar xzv --wildcards 'hub-*/bin/hub' --strip=2
                                ./hub release create --draft --prerelease=${env.TAG_NAME !=~ /v[0-9\.]+/} \\
                                    -a docker-compose-Darwin-x86_64 \\
                                    -a docker-compose-Darwin-x86_64.sha256 \\
                                    -a docker-compose-Darwin-x86_64.tgz \\
                                    -a docker-compose-Darwin-x86_64.tgz.sha256 \\
                                    -a docker-compose-Linux-x86_64 \\
                                    -a docker-compose-Linux-x86_64.sha256 \\
                                    -a docker-compose-Windows-x86_64.exe \\
                                    -a docker-compose-Windows-x86_64.exe.sha256 \\
                                    -a ../script/run/run.sh \\
                                    -F CHANGELOG.md \${TAG_NAME}
                            """)
                        }
                    }
                }
                stage('Publishing Python packages') {
                    agent {
                        label 'linux && docker && ubuntu-2004 && amd64 && cgroup1'
                    }
                    environment {
                        PYPIRC = credentials('pypirc-docker-dsg-cibot')
                    }
                    steps {
                        checkout scm
                        sh """
                            rm -rf build/ dist/
                            pip3 install wheel
                            python3 setup.py sdist bdist_wheel
                            pip3 install twine
                            ~/.local/bin/twine upload --config-file ${PYPIRC} ./dist/docker-compose-*.tar.gz ./dist/docker_compose-*-py2.py3-none-any.whl
                        """
                    }
                }
            }
        }
    }
}


def buildImage(baseImage) {
    def scmvar = checkout(scm)
    def imageName = "dockerpinata/compose:${baseImage}-${scmvar.GIT_COMMIT}"
    image = docker.image(imageName)

    withDockerRegistry(credentialsId:'dockerbuildbot-index.docker.io') {
        try {
            image.pull()
        } catch (Exception exc) {
            ansiColor('xterm') {
                sh """docker build -t ${imageName} \\
                    --target build \\
                    --build-arg DISTRO="${baseImage}" \\
                    --build-arg GIT_COMMIT="${scmvar.GIT_COMMIT}" \\
                    .\\
                """
                sh "docker push ${imageName}"
            }
            echo "${imageName}"
            return imageName
        }
    }
}

def runTests(dockerVersion, pythonVersion, baseImage) {
    return {
        stage("python=${pythonVersion} docker=${dockerVersion} ${baseImage}") {
            node("linux && docker && ubuntu-2004 && amd64 && cgroup1") {
                def scmvar = checkout(scm)
                def imageName = "dockerpinata/compose:${baseImage}-${scmvar.GIT_COMMIT}"
                def storageDriver = sh(script: "docker info -f \'{{.Driver}}\'", returnStdout: true).trim()
                echo "Using local system's storage driver: ${storageDriver}"
                withDockerRegistry(credentialsId:'dockerbuildbot-index.docker.io') {
                    sh """docker run \\
                      -t \\
                      --rm \\
                      --privileged \\
                      --volume="\$(pwd)/.git:/code/.git" \\
                      --volume="/var/run/docker.sock:/var/run/docker.sock" \\
                      --volume="\${DOCKER_CONFIG}/config.json:/root/.docker/config.json" \\
                      -e "DOCKER_TLS_CERTDIR=" \\
                      -e "TAG=${imageName}" \\
                      -e "STORAGE_DRIVER=${storageDriver}" \\
                      -e "DOCKER_VERSIONS=${dockerVersion}" \\
                      -e "BUILD_NUMBER=${env.BUILD_NUMBER}" \\
                      -e "PY_TEST_VERSIONS=${pythonVersion}" \\
                      --entrypoint="script/test/ci" \\
                      ${imageName} \\
                      --verbose
                    """
                }
            }
        }
    }
}

def buildRuntimeImage(baseImage) {
    scmvar = checkout scm
    def imageName = "docker/compose:${baseImage}-${env.BRANCH_NAME}"
    ansiColor('xterm') {
        sh """docker build -t ${imageName} \\
            --build-arg DISTRO="${baseImage}" \\
            --build-arg GIT_COMMIT="${scmvar.GIT_COMMIT.take(7)}" \\
            .
        """
    }
    sh "mkdir -p dist"
    sh "docker save ${imageName} -o dist/docker-compose-${baseImage}.tar"
    stash name: "compose-${baseImage}", includes: "dist/docker-compose-${baseImage}.tar"
}

def pushRuntimeImage(baseImage) {
    unstash "compose-${baseImage}"
    sh "docker load -i dist/docker-compose-${baseImage}.tar"
    withDockerRegistry(credentialsId: 'dockerhub-dockerdsgcibot') {
        sh "docker push docker/compose:${baseImage}-${env.TAG_NAME}"
        if (baseImage == "alpine" && env.TAG_NAME != null) {
            sh "docker tag docker/compose:alpine-${env.TAG_NAME} docker/compose:${env.TAG_NAME}"
            sh "docker push docker/compose:${env.TAG_NAME}"
        }
    }
}

def checksum(filepath) {
    if (isUnix()) {
        sh "openssl sha256 -r -out ${filepath}.sha256 ${filepath}"
    } else {
        powershell "(Get-FileHash -Path ${filepath} -Algorithm SHA256 | % hash).ToLower() + ' *${filepath}' | Out-File -encoding ascii ${filepath}.sha256"
    }
}
