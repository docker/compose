Feature: Down

Scenario: No resources to remove
    When I run "compose down"
    Then the output contains "Warning: No resource found to remove for project "no_resources_to_remove""
