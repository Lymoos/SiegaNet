package net.sieganet.app.core

import net.sieganet.app.api.Status
import net.sieganet.app.api.VpnState
import org.json.JSONObject
import kotlin.random.Random

/**
 * Mock core with the same observable behaviour as the real one: a
 * `connecting` handshake phase, then `connected` with accumulating traffic
 * counters. Lets the whole app run end-to-end (VpnService, notification,
 * tile, timer) before the gomobile .aar exists.
 */
class MockSiegaCore : SiegaCore {

    @Volatile private var state: VpnState = VpnState.DISCONNECTED
    @Volatile private var serverId: String? = null
    @Volatile private var sinceUnix: Long? = null
    @Volatile private var innerIp: String? = null

    private var upBytes = 0.0
    private var downBytes = 0.0
    private var upRate = 0.0
    private var downRate = 0.0
    private var lastTick = 0L
    private var connectAt = 0L
    private var handshakeMs = 0L

    @Synchronized
    override fun start(tunFd: Int, configJson: String) {
        // A real core would take ownership of tunFd here. The mock only
        // simulates the handshake; the fd is owned and closed by the service.
        serverId = runCatching {
            JSONObject(configJson).optString("server_id").ifEmpty { null }
        }.getOrNull()
        state = VpnState.CONNECTING
        connectAt = System.currentTimeMillis()
        handshakeMs = 1300 + Random.nextLong(1200)
        sinceUnix = null
        innerIp = null
        upBytes = 0.0; downBytes = 0.0
    }

    @Synchronized
    override fun stop() {
        state = VpnState.DISCONNECTED
        serverId = null
        sinceUnix = null
        innerIp = null
        upRate = 0.0; downRate = 0.0
    }

    @Synchronized
    override fun statusJson(): String {
        advance()
        return Status(
            state = state,
            serverId = serverId,
            innerIp = innerIp,
            sinceUnix = sinceUnix,
            upBytes = upBytes.toLong(),
            downBytes = downBytes.toLong(),
        ).toJson()
    }

    private fun advance() {
        val now = System.currentTimeMillis()
        if (state == VpnState.CONNECTING && now - connectAt >= handshakeMs) {
            state = VpnState.CONNECTED
            sinceUnix = now / 1000
            innerIp = "10.77.0.${2 + Random.nextInt(200)}"
            upRate = 8_000 + Random.nextDouble() * 30_000
            downRate = 60_000 + Random.nextDouble() * 400_000
            lastTick = now
        }
        if (state == VpnState.CONNECTED) {
            val dt = (now - lastTick) / 1000.0
            lastTick = now
            if (dt > 0) {
                val burst = if (Random.nextDouble() < 0.06) 6.0 else 1.0
                upRate = (upRate * (0.9 + Random.nextDouble() * 0.2)).coerceAtLeast(1_000.0)
                downRate = (downRate * (0.9 + Random.nextDouble() * 0.2)).coerceAtLeast(8_000.0)
                upBytes += upRate * dt * burst * 0.4
                downBytes += downRate * dt * burst
            }
        }
    }
}
