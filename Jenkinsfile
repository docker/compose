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
        TAG = tag()
        BUILD_TAG = tag()
    }

    stages {
        stage('Build test images') {
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

def get_versions(number) {
  def docker_versions
  wrappedNode(label: "ubuntu && amd64 && !zfs") {
    docker_versions = sh(script:"""
        curl https://api.github.com/repos/docker/docker-ce/releases \
         | jq -r -c '.[] | select (.prerelease == false ) | .tag_name | ltrimstr("v")' > /tmp/versions.txt
        for v in \$(cut -f1 -d"." /tmp/versions.txt | uniq | head -${number}); do grep -m 1 "\$v" /tmp/versions.txt ; done
    """, returnStdout: true)
  }
  return docker_versions
}

def runTests = { Map settings ->
  def dockerVersions = settings.get("dockerVersions", null)
  def pythonVersions = settings.get("pythonVersions", null)
  def baseImage = settings.get("baseImage", null)

  if (!pythonVersions) {
    throw new Exception("Need Python versions to test. e.g.: `runTests(pythonVersions: 'py37')`")
  }
  if (!dockerVersions) {
    throw new Exception("Need Docker versions to test. e.g.: `runTests(dockerVersions: 'all')`")
  }

  { ->
    wrappedNode(label: "ubuntu && amd64 && !zfs", cleanWorkspace: true) {
      stage("test python=${pythonVersions} / docker=${dockerVersions} / baseImage=${baseImage}") {
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
          -e "DOCKER_VERSIONS=${dockerVersions}" \\
          -e "BUILD_NUMBER=\$BUILD_TAG" \\
          -e "PY_TEST_VERSIONS=${pythonVersions}" \\
          --entrypoint="script/test/ci" \\
          ${imageName} \\
          --verbose
        """
      }
    }
  }
}

def testMatrix = [failFast: true]
def baseImages = ['alpine', 'debian']
baseImages.each { baseImage ->
  def imageName = buildImage(baseImage)
  get_versions(imageName, 2).eachLine { dockerVersion ->
      testMatrix["${baseImage}_${dockerVersion}"] = runTests([baseImage: baseImage, image: imageName, dockerVersions: dockerVersion, pythonVersions: 'py37'])
  }
}

parallel(testMatrix)
