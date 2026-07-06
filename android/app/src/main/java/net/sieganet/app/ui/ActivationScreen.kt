package net.sieganet.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
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
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import net.sieganet.app.ui.theme.Accent
import net.sieganet.app.ui.theme.AccentLight
import net.sieganet.app.ui.theme.Bg
import net.sieganet.app.ui.theme.Inter
import net.sieganet.app.ui.theme.Mono
import net.sieganet.app.ui.theme.Outfit
import net.sieganet.app.ui.theme.TextMain
import net.sieganet.app.ui.theme.TextMuted

/**
 * Subscription gate: without a valid activation key the VPN does not work.
 * The user enters a key once; it is verified (mock for now) and cached.
 */
@Composable
fun ActivationScreen(
    busy: Boolean,
    error: String?,
    onActivate: (String) -> Unit,
) {
    var key by remember { mutableStateOf("") }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .background(Bg)
            .padding(horizontal = 28.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Text(
            "SiegaNet",
            fontFamily = Outfit,
            fontWeight = FontWeight.Bold,
            fontSize = 26.sp,
            color = TextMain,
        )
        Spacer(Modifier.height(6.dp))
        Text(
            "ТРЕБУЕТСЯ АКТИВАЦИЯ",
            fontFamily = Mono,
            fontSize = 12.sp,
            letterSpacing = 3.sp,
            color = AccentLight,
        )
        Spacer(Modifier.height(10.dp))
        Text(
            "Введите ключ активации подписки.\nБез него подключение недоступно.",
            fontFamily = Inter,
            fontSize = 13.sp,
            color = TextMuted,
            textAlign = TextAlign.Center,
        )

        Spacer(Modifier.height(28.dp))

        OutlinedTextField(
            value = key,
            onValueChange = { key = it },
            enabled = !busy,
            singleLine = true,
            placeholder = {
                Text(
                    "XXXX-XXXX-XXXX-XXXX",
                    fontFamily = Mono,
                    fontSize = 14.sp,
                    color = TextMuted.copy(alpha = 0.6f),
                    modifier = Modifier.fillMaxWidth(),
                    textAlign = TextAlign.Center,
                )
            },
            textStyle = TextStyle(
                fontFamily = Mono,
                fontSize = 15.sp,
                color = TextMain,
                textAlign = TextAlign.Center,
                letterSpacing = 1.sp,
            ),
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
            shape = RoundedCornerShape(12.dp),
            colors = OutlinedTextFieldDefaults.colors(
                focusedBorderColor = Accent,
                unfocusedBorderColor = AccentLight.copy(alpha = 0.25f),
                cursorColor = AccentLight,
            ),
            modifier = Modifier.fillMaxWidth(),
        )

        if (error != null) {
            Spacer(Modifier.height(10.dp))
            Text(
                error,
                fontFamily = Inter,
                fontSize = 12.5.sp,
                color = androidx.compose.ui.graphics.Color(0xFFE08484),
                textAlign = TextAlign.Center,
            )
        }

        Spacer(Modifier.height(18.dp))

        Button(
            onClick = { onActivate(key) },
            enabled = !busy && key.trim().isNotEmpty(),
            shape = RoundedCornerShape(12.dp),
            colors = ButtonDefaults.buttonColors(containerColor = Accent),
            modifier = Modifier
                .fillMaxWidth()
                .height(50.dp),
        ) {
            Text(
                if (busy) "Проверяем…" else "Активировать",
                fontFamily = Inter,
                fontWeight = FontWeight.SemiBold,
                fontSize = 15.sp,
            )
        }
    }
}
