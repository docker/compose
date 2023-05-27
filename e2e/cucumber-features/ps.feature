Feature: PS

Background:
    Given a compose file
        """
        services:
          build:
            image: test:latest
            build:
                context: ./
          pull:
            image: alpine
            command: top
        """
    And a dockerfile
        """
        FROM golang:1.19-alpine
        """
    And I run "docker rm -f external-test"

Scenario: external container from compose image exists
    When I run "compose build"
    Then the exit code is 0
    And I run "docker run --name external-test test:latest ls"
    Then the exit code is 0
    And I run "compose ps -a"
    Then the output does not contain "external-test"
    And I run "docker rm -f external-test"

