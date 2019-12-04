#!groovy

def dockerVersions
def baseImages = ['alpine', 'debian']
def pythonVersions = ['py27', 'py37']

pipeline {
    agent none

    options {
        skipDefaultCheckout(true)
        buildDiscarder(logRotator(daysToKeepStr: '30'))
        timeout(time: 2, unit: 'HOURS')
        timestamps()
    }

    environment {
        TAG = tag()
        BUILD_TAG = tag()
    }

    stages {
        stage('Build test images') {
            // TODO use declarative 1.5.0 `matrix` once available on CI
            parallel {
                stage('alpine') {
                    agent {
                        label 'ubuntu && amd64 && !zfs'
                    }
                    steps {
                        buildImage('alpine')
                    }
                }
                stage('debian') {
                    agent {
                        label 'ubuntu && amd64 && !zfs'
                    }
                    steps {
                        buildImage('debian')
                    }
                }
            }
        }
        stage('Get Docker versions') {
            agent {
                label 'ubuntu'
            }
            steps {
                script {
                    dockerVersions = sh(script:"""
                    curl https://api.github.com/repos/docker/docker-ce/releases \
                        | jq -r -c '.[] | select (.prerelease == false ) | .tag_name | ltrimstr("v")' > /tmp/versions.txt
                    for v in \$(cut -f1 -d"." /tmp/versions.txt | uniq | head -2); do grep -m 1 "\$v" /tmp/versions.txt ; done
                        """, returnStdout: true)
                }
            }
        }
    }
}


def buildImage(baseImage) {
    def scmvar = checkout(scm)
    def imageName = "dockerbuildbot/compose:${baseImage}-${scmvar.GIT_COMMIT}"
    image = docker.image(imageName)

    withDockerRegistry(credentialsId:'dockerbuildbot-index.docker.io') {
        try {
            image.pull()
        } catch (Exception exc) {
            ansiColor('xterm') {
                sh """docker build -t ${imageName} \\
                    --target build \\
                    --build-arg BUILD_PLATFORM="${baseImage}" \\
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
    wrappedNode(label: "ubuntu && amd64 && !zfs", cleanWorkspace: true) {
      stage("test python=${pythonVersion} / docker=${dockerVersion} / baseImage=${baseImage}") {
        def scmvar = checkout(scm)
        def imageName = "dockerbuildbot/compose:${baseImage}-${scmvar.GIT_COMMIT}"
        def storageDriver = sh(script: 'docker info | awk -F \': \' \'$1 == "Storage Driver" { print $2; exit }\'', returnStdout: true).trim()
        echo "Using local system's storage driver: ${storageDriver}"
        sh """docker run \\
          -t \\
          --rm \\
          --privileged \\
          --volume="\$(pwd)/.git:/code/.git" \\
          --volume="/var/run/docker.sock:/var/run/docker.sock" \\
          -e "TAG=${imageName}" \\
          -e "STORAGE_DRIVER=${storageDriver}" \\
          -e "DOCKER_VERSIONS=${dockerVersion}" \\
          -e "BUILD_NUMBER=\$BUILD_TAG" \\
          -e "PY_TEST_VERSIONS=${pythonVersion}" \\
          --entrypoint="script/test/ci" \\
          ${imageName} \\
          --verbose
        """
     }
    }
}

def testMatrix = [failFast: true]

baseImages.each { baseImage ->
  dockerVersions.eachLine { dockerVersion ->
    pythonVersions.each { pythonVersion ->
      testMatrix["${baseImage}_${dockerVersion}_${pythonVersion}"] = runTests(dockerVersion, pythonVersion, baseImage)
    }
  }
}

parallel testMatrix
