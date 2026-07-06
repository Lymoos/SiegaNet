package net.sieganet.app.api

import org.json.JSONObject

/**
 * The same control contract as the desktop client (see desktop/src/api/types.ts
 * and the core control API):
 *
 *   GET  /status  -> { state, server_id, inner_ip, since_unix, up_bytes, down_bytes }
 *   GET  /servers -> [ { id, country, city, host, ping_ms, load_pct } ]
 *
 * On Android the transport differs — the core is linked in as a gomobile
 * module instead of a localhost HTTP server — but the shapes are identical:
 * [SiegaCore.statusJson] returns exactly the GET /status document.
 */

enum class VpnState(val wire: String) {
    DISCONNECTED("disconnected"),
    CONNECTING("connecting"),
    CONNECTED("connected");

    companion object {
        fun fromWire(s: String): VpnState =
            entries.firstOrNull { it.wire == s } ?: DISCONNECTED
    }
}

data class Status(
    val state: VpnState = VpnState.DISCONNECTED,
    val serverId: String? = null,
    val innerIp: String? = null,
    val sinceUnix: Long? = null,
    val upBytes: Long = 0,
    val downBytes: Long = 0,
) {
    companion object {
        fun fromJson(json: String): Status {
            val o = JSONObject(json)
            return Status(
                state = VpnState.fromWire(o.optString("state", "disconnected")),
                serverId = o.optString("server_id").ifEmpty { null },
                innerIp = o.optString("inner_ip").ifEmpty { null },
                sinceUnix = o.optLong("since_unix", 0L).takeIf { it > 0 },
                upBytes = o.optLong("up_bytes", 0),
                downBytes = o.optLong("down_bytes", 0),
            )
        }
    }

    fun toJson(): String = JSONObject().apply {
        put("state", state.wire)
        put("server_id", serverId ?: JSONObject.NULL)
        put("inner_ip", innerIp ?: JSONObject.NULL)
        put("since_unix", sinceUnix ?: JSONObject.NULL)
        put("up_bytes", upBytes)
        put("down_bytes", downBytes)
    }.toString()
}

data class Server(
    val id: String,
    val country: String,
    val city: String,
    val host: String,
    val pingMs: Int,
    val loadPct: Int,
)

/** country name -> flag emoji (same UI-side helper as on desktop) */
fun flagFor(country: String): String {
    val iso = when (country) {
        "Нидерланды" -> "NL"; "Германия" -> "DE"; "Финляндия" -> "FI"
        "Швеция" -> "SE"; "Великобритания" -> "GB"; "Франция" -> "FR"
        "Польша" -> "PL"; "Турция" -> "TR"; "Казахстан" -> "KZ"
        "ОАЭ" -> "AE"; "Сингапур" -> "SG"; "Япония" -> "JP"
        "США" -> "US"; "Бразилия" -> "BR"
        else -> return "🌐"
    }
    val base = 0x1F1E6
    return String(Character.toChars(base + iso[0].code - 'A'.code)) +
        String(Character.toChars(base + iso[1].code - 'A'.code))
}
