class InboundHandler:
    # @gloss implements circ:Replenishment
    def process(self, msg):
        return msg

    # @gloss implements circ:Receiving
    def receive(self):
        pass


class OutboundHandler:
    # @gloss implements circ:WavePlanning
    # @gloss verifies circ:WavePlanning
    def process(self, msg):
        return msg
