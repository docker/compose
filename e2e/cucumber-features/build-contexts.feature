Feature: Build Contexts

Background:
    Given a compose file
        """
        services:
          a:
            build:
                context: .
        """
    And a dockerfile
        """
        # syntax=docker/dockerfile:1
        FROM alpine:latest
        COPY --from=dep /etc/hostname /
        """

Scenario: A scenario
    When I run "compose build --build-context dep=docker-image://ubuntu:latest"
    Then the exit code is 0

