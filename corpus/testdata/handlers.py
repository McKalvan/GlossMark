class InboundHandler:
    # @gloss implements impl:Replenishment
    def process(self, msg):
        return msg

    # @gloss implements impl:Receiving
    def receive(self):
        pass


class OutboundHandler:
    # @gloss implements impl:WavePlanning
    # @gloss verifies impl:WavePlanning
    def process(self, msg):
        return msg
