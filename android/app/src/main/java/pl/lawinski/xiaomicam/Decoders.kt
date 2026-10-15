package pl.lawinski.xiaomicam

import android.media.MediaCodecList
import android.media.MediaFormat

// Decoders: what the hardware video decoders of this device can play. Phones often decode
// H.265 only up to 1920x1088 (Snapdragon 695), so the 2.5K quality of the camera is hidden
// there; an NVIDIA Shield plays it.
object Decoders {
    private val codecs by lazy { MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos.filter { !it.isEncoder } }

    /** codec: ffprobe name (hevc, h264, mjpeg) or "" (not known); size 0: not known */
    fun canPlay(codec: String, w: Int, h: Int): Boolean {
        if (w <= 0 || h <= 0) return true
        val mimes = when (codec) {
            "hevc", "h265" -> listOf(MediaFormat.MIMETYPE_VIDEO_HEVC)
            "h264" -> listOf(MediaFormat.MIMETYPE_VIDEO_AVC)
            "" -> listOf(MediaFormat.MIMETYPE_VIDEO_HEVC, MediaFormat.MIMETYPE_VIDEO_AVC)
            else -> return true // MJPEG and others: let the player try
        }
        return codecs.any { info ->
            mimes.any { mime ->
                info.supportedTypes.any { it.equals(mime, ignoreCase = true) } &&
                    runCatching { info.getCapabilitiesForType(mime).videoCapabilities.isSizeSupported(w, h) }.getOrDefault(false)
            }
        }
    }
}
