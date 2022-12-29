Feature: Up

Background:
    Given a compose file
        """
        services:
          simple:
            image: alpine
            command: top
        """

Scenario: --pull always
    When I run "compose up --pull=always -d"
    Then the output contains "simple Pulled"

