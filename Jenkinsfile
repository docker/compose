#!groovy

def dockerVersions = ''

pipeline {
    agent none

    options {
        skipDefaultCheckout(true)
        buildDiscarder(logRotator(numToKeepStr: '20'))
        timeout(time: 1, unit: 'HOURS')
        parallelsAlwaysFailFast()
    }

    environment {
        TAG = "${env.BUILD_TAG}"
        GOPROXY = "direct"
        DOCKER_BUILDKIT = "1"
    }

    stages {
        stage('Build') {
            parallel {
                stage('Debian') {
                    agent {
                        label 'ubuntu && amd64 && !zfs'
                    }
                    steps {
                        buildImage('debian')
                    }
                }
                stage('Alpine') {
                    agent {
                        label 'ubuntu && amd64 && !zfs'
                    }
                    steps {
                        buildImage('alpine')
                    }
                }
                stage('Docker') {
                    agent {
                        label 'ubuntu && amd64 && !zfs'
                    }
                    steps {
                        script {
                            dockerVersions = sh(script:"""
                                curl -fs https://api.github.com/repos/docker/docker-ce/tags | jq -r ".[].name" | grep "^v[0-9\\.]*\$" > /tmp/versions.txt
                                for v in \$(cut -f1 -d"." /tmp/versions.txt | uniq); do grep -m 1 "\$v" /tmp/versions.txt ; done
                            """, returnStdout: true)
                        }
                        echo "${dockerVersions}"
                    }
                }
            }
        }
        stage('Test') {
            steps{
                script {
                    def testMatrix = [:]
                    def baseImages = ['alpine', 'debian']
                    def pythonVersions = ['py27', 'py37']
                    baseImages.each { baseImage ->
                      dockerVersions.each { dockerVersion ->
                        pythonVersions.each { pyVersion ->
                          testMatrix["${baseImage}_${dockerVersion}_${pyVersion}"] = runTests(baseImage, dockerVersion, pyVersion)
                        }
                      }
                    }
                    parallel testMatrix
                }
            }
        }
    }
}

def buildImage(baseImage) {
    def scmVars = scm
    // https://issues.jenkins-ci.org/browse/JENKINS-26100
    sh """docker build -t compose:${baseImage} --target build  \
         --build-arg BUILD_PLATFORM=${baseImage}              \
         --build-arg GIT_COMMIT=${scmVars.GIT_COMMIT}             \
         ."""
    sh "docker save -o ${baseImage}.tar compose:${baseImage}"
    stash( includes: "${baseImage}.tar", name: "${baseImage}" )
}

def runTests(baseImage, dockerVersion, pyVersion) {
    return node( 'ubuntu && amd64 && !zfs' ) {
        stage("${baseImage} ${dockerVersion} ${pyVersion}") {
           unstash baseImage
            sh "docker load -i ${baseImage}.tar"
            checkout(scm)
            def storageDriver = sh(script: 'docker info | awk -F \': \' \'$1 == "Storage Driver" { print $2; exit }\'', returnStdout: true).trim()
            echo "Using local system's storage driver: ${storageDriver}"
            sh """docker run \\
              -t \\
              --rm \\
              --privileged \\
              --volume="\$(pwd)/.git:/code/.git" \\
              --volume="/var/run/docker.sock:/var/run/docker.sock" \\
              -e "TAG=compose:${baseImage}" \\
              -e "STORAGE_DRIVER=${storageDriver}" \\
              -e "DOCKER_VERSIONS=${dockerVersion}" \\
              -e "BUILD_NUMBER=\$BUILD_TAG" \\
              -e "PY_TEST_VERSIONS=${pyVersion}" \\
              --entrypoint="script/test/ci" \\
              compose:${baseImage} \\
              --verbose
            """
        }
    }
}
