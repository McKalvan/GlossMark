package zoo

// @gloss represents impl:FoalingWindow
const val FOALING_WINDOW_DAYS = 340

// @gloss represents impl:HoofCount
private const val HOOVES = 4

object Stable {
    // @gloss represents impl:StallCount
    val STALLS: Int = 12
}

// @gloss implements impl:FoalRegistry
class FoalRegistry {
    // @gloss implements impl:HoofStep
    fun attach(hoof: Hoof) {
    }
}
