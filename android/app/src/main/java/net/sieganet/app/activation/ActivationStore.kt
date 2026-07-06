package net.sieganet.app.activation

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import kotlin.random.Random

/**
 * Subscription activation (same model as desktop): without a valid activation
 * key the VPN does not work — the connect flow is locked until a key passes
 * POST /activate. The verified key + expiry are cached in encrypted prefs so
 * the user is not asked on every launch.
 *
 * Contract:  POST /activate { key } -> { ok, valid_until }
 *
 * [verify] is the mock of that call (any non-empty key => 30-day validity).
 * The real check goes to the backend later — same request/response shape, so
 * only the body of [verify] changes. In-app purchase, if it ever comes, plugs
 * in as another producer of the same cached (key, valid_until) pair.
 */
object ActivationStore {

    data class Activation(val key: String, val validUntilUnix: Long)

    private const val PREFS = "activation"
    private const val K_KEY = "key"
    private const val K_UNTIL = "valid_until"

    private fun prefs(ctx: Context) = EncryptedSharedPreferences.create(
        ctx,
        PREFS,
        MasterKey.Builder(ctx).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )

    fun load(ctx: Context): Activation? {
        val p = prefs(ctx)
        val key = p.getString(K_KEY, null) ?: return null
        val until = p.getLong(K_UNTIL, 0L)
        return Activation(key, until)
    }

    fun isActivated(ctx: Context): Boolean {
        val a = load(ctx) ?: return false
        return a.validUntilUnix > System.currentTimeMillis() / 1000
    }

    /**
     * Mock of POST /activate. @return the cached activation on success,
     * null when the key is rejected.
     */
    suspend fun verify(ctx: Context, key: String): Activation? =
        withContext(Dispatchers.IO) {
            delay(600 + Random.nextLong(500)) // simulated backend round-trip
            val trimmed = key.trim()
            if (trimmed.isEmpty()) return@withContext null
            val activation = Activation(
                key = trimmed,
                validUntilUnix = System.currentTimeMillis() / 1000 + 30L * 24 * 3600,
            )
            prefs(ctx).edit()
                .putString(K_KEY, activation.key)
                .putLong(K_UNTIL, activation.validUntilUnix)
                .apply()
            activation
        }

    fun clear(ctx: Context) {
        prefs(ctx).edit().clear().apply()
    }
}
