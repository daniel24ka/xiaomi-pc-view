package pl.lawinski.xiaomicam

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

// The look of the desktop app (variant A): graphite, one amber accent, the Geist font.
// Colors are only defined here (cmd/xiaomi-cam/apptheme.go has the same palette).
object Palette {
    val bg = Color(0xFF0E1116)
    val surface = Color(0xFF171B22)
    val raised = Color(0xFF222834)
    val line = Color(0xFF262C38)
    val text = Color(0xFFE8EAEE)
    val muted = Color(0xFF9AA3B2)
    val accent = Color(0xFFF5A524)
    val onAccent = Color(0xFF0E1116)
    val live = Color(0xFF22C55E)
    val rec = Color(0xFFEF4444)
    val overlay = Color(0xB80E1116) // controls lying on the picture
}

val Geist = FontFamily(
    Font(R.font.geist_regular, FontWeight.Normal),
    Font(R.font.geist_semibold, FontWeight.SemiBold),
)

private val colors = darkColorScheme(
    primary = Palette.accent,
    onPrimary = Palette.onAccent,
    secondary = Palette.accent,
    onSecondary = Palette.onAccent,
    background = Palette.bg,
    onBackground = Palette.text,
    surface = Palette.surface,
    onSurface = Palette.text,
    surfaceVariant = Palette.raised,
    onSurfaceVariant = Palette.muted,
    surfaceContainer = Palette.surface,
    surfaceContainerHigh = Palette.raised,
    surfaceContainerHighest = Palette.raised,
    outline = Palette.line,
    outlineVariant = Palette.line,
    error = Palette.rec,
)

private val base = TextStyle(fontFamily = Geist, color = Palette.text)

private val typography = Typography(
    titleLarge = base.copy(fontSize = 22.sp, fontWeight = FontWeight.SemiBold),
    titleMedium = base.copy(fontSize = 17.sp, fontWeight = FontWeight.SemiBold),
    bodyLarge = base.copy(fontSize = 16.sp),
    bodyMedium = base.copy(fontSize = 14.sp),
    bodySmall = base.copy(fontSize = 12.sp, color = Palette.muted),
    labelLarge = base.copy(fontSize = 15.sp, fontWeight = FontWeight.SemiBold),
    labelMedium = base.copy(fontSize = 13.sp, fontWeight = FontWeight.SemiBold),
    labelSmall = base.copy(fontSize = 11.sp),
)

@Composable
fun XcTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = colors, typography = typography, content = content)
}
