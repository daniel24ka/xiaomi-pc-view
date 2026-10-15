package pl.lawinski.xiaomicam

import android.graphics.BitmapFactory
import android.util.Base64
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Computer
import androidx.compose.material.icons.rounded.Login
import androidx.compose.material.icons.rounded.Videocam
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch

/** A screen of a form: centered, scrolls, a title and the content. */
@Composable
fun FormScreen(title: String, content: @Composable () -> Unit) {
    Box(
        Modifier
            .fillMaxSize()
            .background(Palette.bg)
            .safeDrawingPadding(),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            Modifier
                .widthIn(max = 560.dp)
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                .padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(title, style = MaterialTheme.typography.titleLarge)
            content()
        }
    }
}

@Composable
fun BigButton(text: String, icon: androidx.compose.ui.graphics.vector.ImageVector?, primary: Boolean = false, onClick: () -> Unit) {
    Box(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(if (primary) Palette.accent else Palette.surface)
    ) {
        XcButton(
            onClick = onClick, icon = icon, text = text, active = primary, size = SizeLarge,
            shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth(), alignStart = icon != null,
        )
    }
}

@Composable
fun ErrorText(text: String?) {
    if (!text.isNullOrEmpty()) Text(text, color = Palette.rec, style = MaterialTheme.typography.bodyMedium)
}

/** The first screen (no cameras) and "Add a camera" of the menu. */
@Composable
fun WelcomeScreen(onLogin: () -> Unit, onImport: () -> Unit, onIp: () -> Unit, onCancel: (() -> Unit)?) {
    val s = strings()
    FormScreen(if (onCancel == null) s.welcome else s.addCamera) {
        Text(s.welcomeText, style = MaterialTheme.typography.bodyLarge, color = Palette.muted)
        BigButton(s.loginMi, Icons.Rounded.Login, primary = true, onClick = onLogin)
        BigButton(s.importPc, Icons.Rounded.Computer, onClick = onImport)
        Text(s.importPcText, style = MaterialTheme.typography.bodySmall)
        BigButton(s.addIp, Icons.Rounded.Videocam, onClick = onIp)
        if (onCancel != null) BigButton(s.cancel, null, onClick = onCancel)
    }
}

/** Mi account login: user and password, then the captcha or the code when Xiaomi asks. */
@Composable
fun LoginScreen(onDone: () -> Unit, onCancel: () -> Unit) {
    val s = strings()
    val scope = rememberCoroutineScope()
    var user by remember { mutableStateOf("") }
    var pass by remember { mutableStateOf("") }
    var code by remember { mutableStateOf("") }
    var step by remember { mutableStateOf<LoginStep?>(null) } // null: user + password
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }

    fun run(block: suspend () -> LoginStep) {
        busy = true
        error = null
        scope.launch {
            try {
                val r = block()
                if (r.ok) onDone() else {
                    step = r
                    code = ""
                }
            } catch (e: Exception) {
                error = e.message
            } finally {
                busy = false
            }
        }
    }

    FormScreen(s.loginMi) {
        val st = step
        if (st == null) {
            OutlinedTextField(user, { user = it }, Modifier.fillMaxWidth(), label = { Text(s.user) }, singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email))
            OutlinedTextField(pass, { pass = it }, Modifier.fillMaxWidth(), label = { Text(s.password) }, singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password))
        } else {
            if (st.captcha.isNotEmpty()) {
                Text(s.captchaText)
                val bmp = remember(st.captcha) {
                    runCatching { Base64.decode(st.captcha, Base64.DEFAULT) }.getOrNull()
                        ?.let { BitmapFactory.decodeByteArray(it, 0, it.size) }
                }
                if (bmp != null) {
                    Image(bmp.asImageBitmap(), null, Modifier.height(80.dp).background(Palette.text))
                }
            } else {
                Text(s.codeSentTo.format(st.verifyTarget))
            }
            OutlinedTextField(code, { code = it }, Modifier.fillMaxWidth(), singleLine = true)
        }
        ErrorText(error)
        if (busy) CircularProgressIndicator(color = Palette.accent)
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Box(Modifier.weight(1f)) { BigButton(s.cancel, null, onClick = onCancel) }
            Box(Modifier.weight(1f)) {
                BigButton(if (st == null) s.signIn else s.send, null, primary = true) {
                    if (busy) return@BigButton
                    if (st == null) run { Core.login(user, pass) }
                    else run { Core.loginCode(if (st.captcha.isNotEmpty()) "captcha" else "verify", code) }
                }
            }
        }
    }
}

/** The cameras of the Mi account: pick the region, check the cameras, add. */
@Composable
fun DevicesScreen(onAdded: (String) -> Unit, onCancel: () -> Unit) {
    val s = strings()
    val scope = rememberCoroutineScope()
    var accounts by remember { mutableStateOf<List<String>>(emptyList()) }
    var account by remember { mutableStateOf("") }
    var region by remember { mutableStateOf(Core.regions.firstOrNull() ?: "de") }
    val devices = remember { mutableStateListOf<Device>() }
    val checked = remember { mutableStateListOf<Boolean>() }
    var busy by remember { mutableStateOf(true) }
    var info by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(Unit) {
        accounts = Core.accounts()
        account = accounts.firstOrNull() ?: ""
    }
    LaunchedEffect(account, region) {
        if (account.isEmpty()) return@LaunchedEffect
        busy = true
        info = null
        devices.clear()
        checked.clear()
        try {
            val list = Core.devices(account, region)
            devices.addAll(list)
            checked.addAll(list.map { true })
            if (list.isEmpty()) info = s.noCamerasInRegion
        } catch (e: Exception) {
            info = s.error.format(e.message)
        }
        busy = false
    }

    FormScreen(s.addCamera) {
        if (accounts.size > 1) {
            Text(s.account, style = MaterialTheme.typography.bodySmall)
            accounts.forEach { a -> XcButton(onClick = { account = a }, text = a, active = a == account) }
        }
        Text(s.region, style = MaterialTheme.typography.bodySmall)
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            Core.regions.take(3).forEach { r -> XcButton(onClick = { region = r }, text = s.regionNames[r] ?: r, active = r == region) }
        }
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            Core.regions.drop(3).forEach { r -> XcButton(onClick = { region = r }, text = s.regionNames[r] ?: r, active = r == region) }
        }
        if (busy) CircularProgressIndicator(color = Palette.accent)
        info?.let { Text(it, color = Palette.muted) }
        devices.forEachIndexed { i, d ->
            Row(verticalAlignment = Alignment.CenterVertically) {
                Checkbox(checked.getOrElse(i) { false }, { checked[i] = it })
                Column {
                    Text(d.name, style = MaterialTheme.typography.titleMedium)
                    Text(d.info, style = MaterialTheme.typography.bodySmall)
                }
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Box(Modifier.weight(1f)) { BigButton(s.cancel, null, onClick = onCancel) }
            Box(Modifier.weight(1f)) {
                BigButton(s.add, null, primary = true) {
                    scope.launch {
                        var first: String? = null
                        try {
                            devices.forEachIndexed { i, d ->
                                if (checked.getOrElse(i) { false }) {
                                    val name = Core.addXiaomi(d)
                                    if (first == null) first = name
                                }
                            }
                            first?.let(onAdded)
                        } catch (e: Exception) {
                            info = s.error.format(e.message)
                        }
                    }
                }
            }
        }
    }
}

/** An IP camera by its address (RTSP, HTTP/MJPEG/HLS, ONVIF, Tapo, DVRIP). */
@Composable
fun IpCameraScreen(onAdded: (String) -> Unit, onCancel: () -> Unit) {
    val s = strings()
    val scope = rememberCoroutineScope()
    var name by remember { mutableStateOf("") }
    var url by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    FormScreen(s.addIp) {
        OutlinedTextField(name, { name = it }, Modifier.fillMaxWidth(), label = { Text(s.ipName) }, singleLine = true)
        OutlinedTextField(url, { url = it }, Modifier.fillMaxWidth(), label = { Text(s.ipUrl) },
            placeholder = { Text(s.ipUrlHint) }, singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri))
        ErrorText(error)
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Box(Modifier.weight(1f)) { BigButton(s.cancel, null, onClick = onCancel) }
            Box(Modifier.weight(1f)) {
                BigButton(s.add, null, primary = true) {
                    scope.launch {
                        try {
                            onAdded(Core.addIPCamera(name, url))
                        } catch (e: Exception) {
                            error = e.message
                        }
                    }
                }
            }
        }
    }
}

@Composable
fun LoadingScreen() {
    Box(Modifier.fillMaxSize().background(Palette.bg), contentAlignment = Alignment.Center) {
        CircularProgressIndicator(color = Palette.accent, modifier = Modifier.size(48.dp))
    }
}
