package pl.lawinski.xiaomicam

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.focusable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Videocam
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

// GridScreen - all cameras at once, like the grid of the desktop app. Each tile plays its
// camera (one go2rtc session per camera: the live view of the same camera shares it). Tap
// (or OK on the remote) opens the camera alone. Columns: upright phone 1-2, landscape /
// TV / car radio 2-3.
@Composable
fun GridScreen(cameras: List<Camera>, onOpen: (Camera) -> Unit, onSingle: () -> Unit, onMenu: () -> Unit) {
    val s = strings()
    BoxWithConstraints(
        Modifier
            .fillMaxSize()
            .background(Palette.bg)
            .safeDrawingPadding()
    ) {
        val portrait = maxHeight > maxWidth
        val n = cameras.size
        val cols = when {
            portrait -> if (n <= 2) 1 else 2
            n <= 4 -> 2
            else -> 3
        }
        Column(Modifier.fillMaxSize()) {
            Row(
                Modifier.fillMaxWidth().padding(12.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(s.cameras, style = MaterialTheme.typography.titleLarge, modifier = Modifier.padding(start = 8.dp))
                Spacer(Modifier.weight(1f))
                CompositionLocalPill {
                    XcButton(onClick = onSingle, icon = Icons.Rounded.Videocam, label = s.oneCamera)
                }
                CompositionLocalPill {
                    XcButton(onClick = onMenu, icon = Icons.Rounded.MoreVert, label = s.menu)
                }
            }
            Column(
                Modifier
                    .fillMaxSize()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 12.dp, vertical = 4.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                cameras.chunked(cols).forEach { row ->
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        row.forEach { c -> GridTile(c, Modifier.weight(1f)) { onOpen(c) } }
                        repeat(cols - row.size) { Spacer(Modifier.weight(1f)) }
                    }
                }
            }
        }
    }
}

@Composable
private fun CompositionLocalPill(content: @Composable () -> Unit) {
    Box(Modifier.clip(RoundedCornerShape(50)).background(Palette.surface).padding(4.dp)) { content() }
}

@Composable
private fun GridTile(camera: Camera, modifier: Modifier, onOpen: () -> Unit) {
    var status by remember { mutableStateOf(VideoStatus.CONNECTING) }
    val interaction = remember { MutableInteractionSource() }
    val focused by interaction.collectIsFocusedAsState()
    val shape = RoundedCornerShape(12.dp)
    Box(
        modifier
            .aspectRatio(16f / 9f)
            .clip(shape)
            .background(Palette.surface)
            .border(2.dp, if (focused) Palette.accent else Color.Transparent, shape)
            .onKeyEvent {
                if (it.type == KeyEventType.KeyUp && (it.key == Key.DirectionCenter || it.key == Key.Enter)) {
                    onOpen(); true
                } else false
            }
            .focusable(interactionSource = interaction)
            .clickable(interactionSource = interaction, indication = null, onClick = onOpen),
    ) {
        CameraPlayer(Core.rtspUrl(camera.name), camera.name, Modifier.fillMaxSize()) { st, _ -> status = st }
        Row(
            Modifier
                .padding(8.dp)
                .clip(RoundedCornerShape(50))
                .background(Palette.overlay)
                .padding(horizontal = 10.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            val color = when (status) {
                VideoStatus.PLAYING -> Palette.live
                VideoStatus.CONNECTING -> Palette.accent
                VideoStatus.ERROR -> Palette.rec
            }
            Box(Modifier.size(8.dp).clip(CircleShape).background(color))
            Spacer(Modifier.width(6.dp))
            Text(camera.name, style = MaterialTheme.typography.labelMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
    }
}
