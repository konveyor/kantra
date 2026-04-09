"""Sample sources for python provider tests (python-sample-rule-001–004)."""


def hello_world() -> None:
    """Referenced by python-sample-rule-001."""
    pass


def speak() -> None:
    """Referenced by python-sample-rule-002."""
    pass


def create_custom_resource_definition() -> None:
    """Referenced by python-sample-rule-003."""
    pass


def file_backup() -> None:
    """Referenced by python-sample-rule-004 (pattern file_b*)."""
    pass
