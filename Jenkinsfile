#!groovy

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
        stage('one') {
            matrix {
                axes {
                    axis {
                        name 'baseImage'
                        values 'alpine', 'debian'
                    }
                }
                stages {
                    stage("Build test images") {
                        agent {
                            label 'ubuntu-2004 && amd64 && !zfs && cgroup1'
                        }
                        steps {
                            buildImage(baseImage)
                        }
                    }
                }
            }
        }
        stage('two') {
            matrix {
                axes {
                    axis {
                        name 'dockerVersion'
                        values '19.03.13'
                    }
                    axis {
                        name 'pythonVersion' 
                        values 'py37'
                    }
                    axis {
                        name 'baseImage'
                        values 'alpine', 'debian'
                    }
                }
                stages {
                    stage("Test") {
                        agent {
                            label "ubuntu-2004 && amd64 && !zfs && cgroup1"
                        }
                        steps {
                            script {
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
