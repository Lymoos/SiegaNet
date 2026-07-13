package net.sieganet.app.ui

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import net.sieganet.app.R
import net.sieganet.app.ui.theme.Accent
import net.sieganet.app.ui.theme.AccentDeep
import net.sieganet.app.ui.theme.AccentLight
import net.sieganet.app.ui.theme.Bg
import net.sieganet.app.ui.theme.Inter
import net.sieganet.app.ui.theme.Mono
import net.sieganet.app.ui.theme.Outfit
import net.sieganet.app.ui.theme.Panel
import net.sieganet.app.ui.theme.TextMain
import net.sieganet.app.ui.theme.TextMuted

/**
 * Account gate: the app requires a SiegaNet account. Logging in reaches the
 * app; a subscription is what unlocks connecting (handled in the profile).
 */
@Composable
fun LoginScreen(
    busy: Boolean,
    error: String?,
    onLogin: (String, String) -> Unit,
) {
    var email by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Bg),
        contentAlignment = Alignment.Center,
    ) {
        Aurora()

        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 28.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            FloatingGlyph()
            Spacer(Modifier.height(12.dp))
            Text("SiegaNet", fontFamily = Outfit, fontWeight = FontWeight.Bold, fontSize = 26.sp, color = TextMain)
            Text(
                "STEALTH VPN",
                fontFamily = Mono, fontSize = 10.sp, letterSpacing = 3.sp, color = TextMuted,
            )

            Spacer(Modifier.height(26.dp))
            Text("Вход в аккаунт", fontFamily = Inter, fontWeight = FontWeight.SemiBold, fontSize = 17.sp, color = TextMain)
            Spacer(Modifier.height(18.dp))

            Field(
                value = email,
                onValueChange = { email = it },
                label = "Почта",
                placeholder = "you@example.com",
                enabled = !busy,
                keyboard = KeyboardOptions(keyboardType = KeyboardType.Email, imeAction = ImeAction.Next),
            )
            Spacer(Modifier.height(12.dp))
            Field(
                value = password,
                onValueChange = { password = it },
                label = "Пароль",
                placeholder = "••••••••",
                enabled = !busy,
                password = true,
                keyboard = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done),
            )

            if (error != null) {
                Spacer(Modifier.height(10.dp))
                Text(error, fontFamily = Inter, fontSize = 12.5.sp, color = Color(0xFFE08484), textAlign = TextAlign.Center)
            }

            Spacer(Modifier.height(20.dp))
            Button(
                onClick = { onLogin(email, password) },
                enabled = !busy && email.trim().isNotEmpty() && password.isNotEmpty(),
                shape = RoundedCornerShape(12.dp),
                colors = ButtonDefaults.buttonColors(containerColor = Accent),
                modifier = Modifier
                    .fillMaxWidth()
                    .height(50.dp),
            ) {
                Text(if (busy) "Входим…" else "Войти", fontFamily = Inter, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
            }

            Spacer(Modifier.height(16.dp))
            Text(
                "Нет аккаунта? Регистрация появится в ближайшем обновлении.",
                fontFamily = Inter, fontSize = 11.5.sp, color = TextMuted, textAlign = TextAlign.Center,
            )
        }
    }
}

@Composable
private fun Aurora() {
    val t = rememberInfiniteTransition(label = "aurora")
    val shift by t.animateFloat(
        initialValue = 0f, targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(9000), RepeatMode.Reverse),
        label = "shift",
    )
    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(
                Brush.radialGradient(
                    colors = listOf(Accent.copy(alpha = 0.20f), Color.Transparent),
                    center = Offset(300f + shift * 260f, 500f + shift * 180f),
                    radius = 720f,
                ),
            )
            .background(
                Brush.radialGradient(
                    colors = listOf(AccentDeep.copy(alpha = 0.22f), Color.Transparent),
                    center = Offset(900f - shift * 240f, 1500f - shift * 200f),
                    radius = 680f,
                ),
            ),
    )
}

@Composable
private fun FloatingGlyph() {
    val t = rememberInfiniteTransition(label = "glyph")
    val spin by t.animateFloat(
        initialValue = 0f, targetValue = 360f,
        animationSpec = infiniteRepeatable(tween(6000), RepeatMode.Restart),
        label = "spin",
    )
    Box(contentAlignment = Alignment.Center) {
        // faint orbiting arc around the brand glyph
        androidx.compose.foundation.Canvas(
            modifier = Modifier
                .size(72.dp)
                .rotate(spin),
        ) {
            drawArc(
                color = AccentLight.copy(alpha = 0.55f),
                startAngle = 0f, sweepAngle = 90f, useCenter = false,
                style = Stroke(width = 2.5f),
            )
        }
        Box(
            modifier = Modifier
                .size(52.dp)
                .background(Brush.linearGradient(listOf(Accent, AccentDeep)), RoundedCornerShape(15.dp)),
            contentAlignment = Alignment.Center,
        ) {
            Icon(painterResource(R.drawable.ic_shield), contentDescription = null, tint = Color.White, modifier = Modifier.size(24.dp))
        }
    }
}

@Composable
private fun Field(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    placeholder: String,
    enabled: Boolean,
    password: Boolean = false,
    keyboard: KeyboardOptions = KeyboardOptions.Default,
) {
    Column(Modifier.fillMaxWidth()) {
        Text(label, fontFamily = Inter, fontSize = 11.5.sp, color = TextMuted, modifier = Modifier.padding(bottom = 6.dp))
        OutlinedTextField(
            value = value,
            onValueChange = onValueChange,
            enabled = enabled,
            singleLine = true,
            placeholder = { Text(placeholder, fontFamily = Inter, fontSize = 14.sp, color = TextMuted.copy(alpha = 0.5f)) },
            visualTransformation = if (password) PasswordVisualTransformation() else androidx.compose.ui.text.input.VisualTransformation.None,
            keyboardOptions = keyboard,
            shape = RoundedCornerShape(12.dp),
            colors = OutlinedTextFieldDefaults.colors(
                focusedBorderColor = Accent,
                unfocusedBorderColor = AccentLight.copy(alpha = 0.25f),
                focusedContainerColor = Panel,
                unfocusedContainerColor = Panel,
                cursorColor = AccentLight,
                focusedTextColor = TextMain,
                unfocusedTextColor = TextMain,
            ),
            modifier = Modifier.fillMaxWidth(),
        )
    }
}
