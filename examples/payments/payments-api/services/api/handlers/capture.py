# services/api/handlers/capture.py
# @gloss implements .CaptureHandler
def capture(payment_id: str) -> None: ...


# @gloss implements .CaptureEmitter
def emit_capture_confirmed(payment_id: str) -> None: ...
