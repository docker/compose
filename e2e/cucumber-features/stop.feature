Feature: Stop

Background:
    Given a compose file
        """
        services:
          should_fail:
            image: alpine
            command: ls /does_not_exist
          sleep: # will be killed
            image: alpine
            command: ping localhost
        """

Scenario: Cascade stop
    When I run "compose up --abort-on-container-exit"
    Then the output contains "should_fail-1 exited with code 1"
    And the output contains "Aborting on container exit..."
    And the exit code is 1

Scenario: Exit code from
    When I run "compose up --exit-code-from sleep"
    Then the output contains "should_fail-1 exited with code 1"
    And the output contains "Aborting on container exit..."
    And the exit code is 137

Scenario: Exit code from unknown service
    When I run "compose up --exit-code-from unknown"
    Then the output contains "no such service: unknown"
    And the exit code is 1
