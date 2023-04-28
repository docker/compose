Feature: Build Contexts

Background:
    Given a compose file
        """
        services:
          a:
            build:
                context: .
                dockerfile_inline: |
                    # syntax=docker/dockerfile:1
                    FROM alpine:latest
                    COPY --from=dep /etc/hostname /
                additional_contexts:
                  - dep=docker-image://ubuntu:latest
        """

Scenario: Build w/ build context
    When I run "compose build"
    Then the exit code is 0

