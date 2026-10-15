package pl.lawinski.xiaomicam

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.waitForUpOrCancellation
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.KeyboardArrowDown
import androidx.compose.material.icons.rounded.KeyboardArrowLeft
import androidx.compose.material.icons.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.KeyboardArrowUp
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

// Controls on the picture: pills on a translucent background, like the desktop overlay.
// Every control works with touch and with a remote (D-pad focus: an amber ring).

/** sizes of the controls: bigger on a car radio / TV (seen from a distance, touched while driving) */
data class ControlSize(val button: Dp, val icon: Dp, val gap: Dp)

val SizeNormal = ControlSize(44.dp, 22.dp, 6.dp)
val SizeLarge = ControlSize(60.dp, 30.dp, 10.dp)

/** the background of the pills: translucent on the picture, solid in a control panel */
val LocalPillColor = staticCompositionLocalOf { Palette.overlay }

@Composable
fun Pill(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    Box(
        modifier
            .clip(RoundedCornerShape(50))
            .background(LocalPillColor.current)
            .padding(4.dp),
    ) { content() }
}

/**
 * A round button that reports press and release (moves the camera while held). Touch:
 * finger down/up; remote: the center key down/up while focused.
 */
@Composable
fun HoldButton(
    icon: ImageVector,
    label: String,
    size: ControlSize,
    onPress: () -> Unit,
    onRelease: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val interaction = remember { MutableInteractionSource() }
    val focused by interaction.collectIsFocusedAsState()
    Box(
        modifier
            .size(size.button)
            .clip(CircleShape)
            .background(if (focused) Palette.raised else Color.Transparent)
            .border(2.dp, if (focused) Palette.accent else Color.Transparent, CircleShape)
            .semantics { contentDescription = label; role = Role.Button }
            .onKeyEvent {
                if (it.key != Key.DirectionCenter && it.key != Key.Enter && it.key != Key.NumPadEnter) {
                    return@onKeyEvent false
                }
                when (it.type) {
                    KeyEventType.KeyDown -> onPress()
                    KeyEventType.KeyUp -> onRelease()
                }
                true
            }
            .focusable(interactionSource = interaction)
            .pointerInput(Unit) {
                awaitEachGesture {
                    awaitFirstDown()
                    onPress()
                    waitForUpOrCancellation()
                    onRelease()
                }
            },
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, null, Modifier.size(size.icon), tint = Palette.text)
    }
}

/** A button: an icon and/or a text; active - amber (a switched on light, the selected quality). */
@Composable
fun XcButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    icon: ImageVector? = null,
    text: String? = null,
    label: String = text ?: "",
    active: Boolean = false,
    size: ControlSize = SizeNormal,
    shape: Shape = RoundedCornerShape(50),
    enabled: Boolean = true,
    alignStart: Boolean = false,
) {
    val interaction = remember { MutableInteractionSource() }
    val focused by interaction.collectIsFocusedAsState()
    val fg = when {
        !enabled -> Palette.muted.copy(alpha = 0.5f)
        active -> Palette.onAccent
        else -> Palette.text
    }
    Row(
        modifier
            .clip(shape)
            .background(
                when {
                    active -> Palette.accent
                    focused -> Palette.raised
                    else -> Color.Transparent
                }
            )
            .border(2.dp, if (focused) Palette.accent else Color.Transparent, shape)
            .semantics { contentDescription = label; role = Role.Button }
            .onKeyEvent {
                if (enabled && it.type == KeyEventType.KeyUp &&
                    (it.key == Key.DirectionCenter || it.key == Key.Enter || it.key == Key.NumPadEnter)
                ) {
                    onClick(); true
                } else false
            }
            .focusable(enabled, interaction)
            .pointerInput(enabled) {
                if (!enabled) return@pointerInput
                awaitEachGesture {
                    awaitFirstDown()
                    if (waitForUpOrCancellation() != null) onClick()
                }
            }
            .padding(horizontal = if (text != null) size.gap * 2 else 0.dp)
            .let { if (text == null) it.size(size.button) else it.padding(vertical = size.gap) },
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = if (alignStart) Arrangement.Start else Arrangement.Center,
    ) {
        if (icon != null) Icon(icon, null, Modifier.size(size.icon), tint = fg)
        if (icon != null && text != null) Spacer(Modifier.width(size.gap))
        if (text != null) {
            Text(
                text,
                color = fg,
                style = if (size == SizeLarge) MaterialTheme.typography.titleMedium else MaterialTheme.typography.labelLarge,
                maxLines = 1,
            )
        }
    }
}

/** The pan/tilt pad: four hold buttons in a cross. */
@Composable
fun PtzPad(camera: String, size: ControlSize, modifier: Modifier = Modifier) {
    val release = { Core.motorRelease() }
    Box(
        modifier
            .clip(CircleShape)
            .background(LocalPillColor.current)
            .padding(4.dp)
    ) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            HoldButton(Icons.Rounded.KeyboardArrowUp, strings().up, size, { Core.motorPress(camera, Core.MOTOR_UP) }, release)
            Row {
                HoldButton(Icons.Rounded.KeyboardArrowLeft, strings().left, size, { Core.motorPress(camera, Core.MOTOR_LEFT) }, release)
                Spacer(Modifier.size(size.button))
                HoldButton(Icons.Rounded.KeyboardArrowRight, strings().right, size, { Core.motorPress(camera, Core.MOTOR_RIGHT) }, release)
            }
            HoldButton(Icons.Rounded.KeyboardArrowDown, strings().down, size, { Core.motorPress(camera, Core.MOTOR_DOWN) }, release)
        }
    }
}
