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
    And the output contains "simple Pulled"
    Then I run "compose up --pull=always -d"
    And the output contains "simple Pulled"

