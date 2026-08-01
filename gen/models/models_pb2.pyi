from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional

DESCRIPTOR: _descriptor.FileDescriptor

class ClassifyRequest(_message.Message):
    __slots__ = ("query",)
    QUERY_FIELD_NUMBER: _ClassVar[int]
    query: str
    def __init__(self, query: _Optional[str] = ...) -> None: ...

class ClassifyResponse(_message.Message):
    __slots__ = ("intent", "confidence", "runner_up")
    INTENT_FIELD_NUMBER: _ClassVar[int]
    CONFIDENCE_FIELD_NUMBER: _ClassVar[int]
    RUNNER_UP_FIELD_NUMBER: _ClassVar[int]
    intent: str
    confidence: float
    runner_up: str
    def __init__(self, intent: _Optional[str] = ..., confidence: _Optional[float] = ..., runner_up: _Optional[str] = ...) -> None: ...

class RelevanceRequest(_message.Message):
    __slots__ = ("query", "snippet")
    QUERY_FIELD_NUMBER: _ClassVar[int]
    SNIPPET_FIELD_NUMBER: _ClassVar[int]
    query: str
    snippet: str
    def __init__(self, query: _Optional[str] = ..., snippet: _Optional[str] = ...) -> None: ...

class RelevanceResponse(_message.Message):
    __slots__ = ("score",)
    SCORE_FIELD_NUMBER: _ClassVar[int]
    score: float
    def __init__(self, score: _Optional[float] = ...) -> None: ...
