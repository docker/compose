#!groovy

def dockerVersions = ['19.03.5', '18.09.9']
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

    stages {
        stage('Build test images') {
            // TODO use declarative 1.5.0 `matrix` once available on CI
            parallel {
                stage('alpine') {
                    agent {
                        label 'linux'
                    }
                    steps {
                        buildImage('alpine')
                    }
                }
                stage('debian') {
                    agent {
                        label 'linux'
                    }
                    steps {
                        buildImage('debian')
                    }
                }
            }
        }
        stage('Test') {
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
                label 'linux'
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
                    steps {
                        checkout scm
                        sh './script/setup/osx'
                        sh 'tox -e py27,py37 -- tests/unit'
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
                        label 'linux'
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
                        PATH = "$PATH;C:\\Python37;C:\\Python37\\Scripts"
                    }
                    steps {
                        checkout scm
                        bat 'tox.exe -e py27,py37 -- tests/unit'
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
                        label 'linux'
                    }
                    steps {
                        buildRuntimeImage('alpine')
                    }
                }
                stage('debian image') {
                    agent {
                        label 'linux'
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
                        label 'linux'
                    }
                    steps {
                        pushRuntimeImage('alpine')
                        pushRuntimeImage('debian')
                    }
                }
                stage('Creating Github Release') {
                    agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        sh 'mkdir -p dist'
                        dir("dist") {
                            unstash "bin-darwin"
                            unstash "bin-linux"
                            unstash "bin-win"
                            unstash "changelog"
                            githubRelease()
                        }
                    }
                }
                stage('Publishing Python packages') {
                    agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        withCredentials([[$class: "FileBinding", credentialsId: 'pypirc-docker-dsg-cibot', variable: 'PYPIRC']]) {
                            sh """
                                virtualenv venv-publish
                                source venv-publish/bin/activate
                                python setup.py sdist bdist_wheel
                                pip install twine
                                twine upload --config-file ${PYPIRC} ./dist/docker-compose-${env.TAG_NAME}.tar.gz ./dist/docker_compose-${env.TAG_NAME}-py2.py3-none-any.whl
                            """
                        }
                    }
                    post {
                        always {
                            sh 'deactivate; rm -rf venv-publish'
                        }
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

def runTests(dockerVersion, pythonVersion, baseImage) {
    return {
        stage("python=${pythonVersion} docker=${dockerVersion} ${baseImage}") {
            node("linux") {
                def scmvar = checkout(scm)
                def imageName = "dockerbuildbot/compose:${baseImage}-${scmvar.GIT_COMMIT}"
                def storageDriver = sh(script: "docker info -f \'{{.Driver}}\'", returnStdout: true).trim()
                echo "Using local system's storage driver: ${storageDriver}"
                withDockerRegistry(credentialsId:'dockerbuildbot-index.docker.io') {
                    sh """docker run \\
                      -t \\
                      --rm \\
                      --privileged \\
                      --volume="\$(pwd)/.git:/code/.git" \\
                      --volume="/var/run/docker.sock:/var/run/docker.sock" \\
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
            --build-arg BUILD_PLATFORM="${baseImage}" \\
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
    sh 'echo -n "${DOCKERHUB_CREDS_PSW}" | docker login --username "${DOCKERHUB_CREDS_USR}" --password-stdin'
    sh "docker load -i dist/docker-compose-${baseImage}.tar"
    withDockerRegistry(credentialsId: 'dockerbuildbot-hub.docker.com') {
        sh "docker push docker/compose:${baseImage}-${env.TAG_NAME}"
        if (baseImage == "alpine" && env.TAG_NAME != null) {
            sh "docker tag docker/compose:alpine-${env.TAG_NAME} docker/compose:${env.TAG_NAME}"
            sh "docker push docker/compose:${env.TAG_NAME}"
        }
    }
}

def githubRelease() {
    withCredentials([string(credentialsId: 'github-compose-release-test-token', variable: 'GITHUB_TOKEN')]) {
        def prerelease = !( env.TAG_NAME ==~ /v[0-9\.]+/ )
        changelog = readFile "CHANGELOG.md"
        def data = """{
            \"tag_name\": \"${env.TAG_NAME}\",
            \"name\": \"${env.TAG_NAME}\",
            \"draft\": true,
            \"prerelease\": ${prerelease},
            \"body\" : \"${changelog}\"
        """
        echo $data

        def url = "https://api.github.com/repos/docker/compose/releases"
        def upload_url = sh(returnStdout: true, script: """
            curl -sSf -H 'Authorization: token ${GITHUB_TOKEN}' -H 'Accept: application/json' -H 'Content-type: application/json' -X POST -d '$data' $url") \\
            | jq '.upload_url | .[:rindex("{")]'
        """)
        sh("""
            for f in * ; do
                curl -sf -H 'Authorization: token ${GITHUB_TOKEN}' -H 'Accept: application/json' -H 'Content-type: application/octet-stream' \\
                -X POST --data-binary @\$f ${upload_url}?name=\$f;
            done
        """)
    }
}

def checksum(filepath) {
    if (isUnix()) {
        sh "openssl sha256 -r -out ${filepath}.sha256 ${filepath}"
    } else {
        powershell "(Get-FileHash -Path ${filepath} -Algorithm SHA256 | % hash) + ' *${filepath}' > ${filepath}.sha256"
    }
}
