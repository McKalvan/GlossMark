# @gloss implements .FoalingPipelineThroughput
def throughput(window) -> float:
    """Foals per day over the window; published to the business as ProductionCapacity."""
    return window.count() / window.days()
