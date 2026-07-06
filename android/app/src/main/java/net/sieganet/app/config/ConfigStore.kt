package net.sieganet.app.config

import android.content.Context
import android.net.Uri
import android.util.Base64
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import org.json.JSONObject

/**
 * Imported peer config, at rest in EncryptedSharedPreferences.
 *
 * Import sources (both carry the same payload):
 *   - deep link  sieganet://import?c=<base64url(config)>
 *   - QR code containing that same sieganet:// URI (or the raw config text)
 *
 * The payload is the client config as issued by `sieganet-ctl add` (TOML or
 * its JSON equivalent: server, peer_id, psk, tunnel_path, mtu…). The UI never
 * interprets the secret fields — it stores the blob and hands it verbatim to
 * the core on start.
 */
object ConfigStore {

    private const val PREFS = "peer_config"
    private const val KEY_RAW = "raw"

    private fun prefs(ctx: Context) = EncryptedSharedPreferences.create(
        ctx,
        PREFS,
        MasterKey.Builder(ctx).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )

    /** @return human summary («peer @ host») on success, null if unparseable */
    fun importFrom(ctx: Context, text: String): String? {
        val raw = extractPayload(text) ?: return null
        prefs(ctx).edit().putString(KEY_RAW, raw).apply()
        return summary(ctx)
    }

    fun hasConfig(ctx: Context): Boolean = rawConfig(ctx) != null

    fun rawConfig(ctx: Context): String? = prefs(ctx).getString(KEY_RAW, null)

    /** short display string for the UI, no secrets */
    fun summary(ctx: Context): String? {
        val raw = rawConfig(ctx) ?: return null
        val peer = pick(raw, "peer_id")
        val server = pick(raw, "server")
        return when {
            peer != null && server != null -> "$peer @ $server"
            server != null -> server
            else -> "конфиг импортирован"
        }
    }

    /**
     * Core-facing config document. The raw import is normalised to JSON:
     * TOML `key = "value"` lines are lifted as-is (good enough for the flat
     * client.toml shape), JSON payloads pass through.
     */
    fun asCoreConfigJson(ctx: Context): String {
        val raw = rawConfig(ctx) ?: return "{}"
        val trimmed = raw.trim()
        if (trimmed.startsWith("{")) return trimmed
        val o = JSONObject()
        for (key in listOf("server", "peer_id", "psk", "tunnel_path")) {
            pick(raw, key)?.let { o.put(key, it) }
        }
        Regex("""(?m)^\s*mtu\s*=\s*(\d+)""").find(raw)?.let {
            o.put("mtu", it.groupValues[1].toInt())
        }
        return o.toString()
    }

    private fun extractPayload(text: String): String? {
        val t = text.trim()
        if (t.startsWith("sieganet://", ignoreCase = true)) {
            val uri = Uri.parse(t)
            val b64 = uri.getQueryParameter("c") ?: uri.getQueryParameter("config")
            return b64?.let {
                runCatching {
                    String(Base64.decode(it, Base64.URL_SAFE or Base64.NO_WRAP))
                }.getOrNull()
            }
        }
        // raw TOML/JSON straight from a QR code
        return t.takeIf { it.contains("server") || it.startsWith("{") }
    }

    private fun pick(raw: String, key: String): String? =
        Regex("""(?m)^\s*"?$key"?\s*[=:]\s*"([^"]+)"""").find(raw)?.groupValues?.get(1)
}
