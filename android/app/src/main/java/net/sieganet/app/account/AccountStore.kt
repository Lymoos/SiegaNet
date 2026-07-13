package net.sieganet.app.account

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import kotlin.random.Random

/**
 * Account + subscription (same model as desktop). Accounts live on the
 * backend; the UI requires login and gates connecting on an active
 * subscription. Subscription codes are redeemed inside the profile, not as a
 * pre-app gate. Session + subscription are cached in encrypted prefs so the
 * user isn't asked to log in every launch.
 *
 * Contract (backend, mocked here — real HTTP later, same shapes):
 *   POST /auth/login     { email, password } -> { ok, token, account }
 *   POST /account/redeem { code }            -> { ok, valid_until, plan }
 *
 * In-app purchase, when it eventually arrives, plugs in as another producer
 * of the same cached (plan, validUntil) pair.
 */
object AccountStore {

    data class Account(
        val email: String,
        val token: String,
        val plan: String,
        val validUntilUnix: Long,
    ) {
        val hasActiveSubscription: Boolean
            get() = validUntilUnix > System.currentTimeMillis() / 1000
    }

    private const val PREFS = "account"
    private const val K_EMAIL = "email"
    private const val K_TOKEN = "token"
    private const val K_PLAN = "plan"
    private const val K_UNTIL = "valid_until"

    private fun prefs(ctx: Context) = EncryptedSharedPreferences.create(
        ctx,
        PREFS,
        MasterKey.Builder(ctx).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )

    fun load(ctx: Context): Account? {
        val p = prefs(ctx)
        val email = p.getString(K_EMAIL, null) ?: return null
        val token = p.getString(K_TOKEN, null) ?: return null
        return Account(
            email = email,
            token = token,
            plan = p.getString(K_PLAN, "—") ?: "—",
            validUntilUnix = p.getLong(K_UNTIL, 0L),
        )
    }

    fun isLoggedIn(ctx: Context): Boolean = load(ctx) != null

    fun hasActiveSubscription(ctx: Context): Boolean =
        load(ctx)?.hasActiveSubscription == true

    /**
     * Mock of POST /auth/login. Any valid email + non-trivial password
     * succeeds, returning an account with NO active subscription (so the
     * redeem flow is visible). @return the account on success, else the
     * error message.
     */
    suspend fun login(ctx: Context, email: String, password: String): Result<Account> =
        withContext(Dispatchers.IO) {
            delay(650 + Random.nextLong(300))
            val e = email.trim()
            if (!Regex("^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$").matches(e)) {
                return@withContext Result.failure(Exception("Неверный формат почты"))
            }
            if (password.length < 4) {
                return@withContext Result.failure(Exception("Пароль слишком короткий"))
            }
            val account = Account(
                email = e,
                token = "mock-${e.hashCode()}",
                plan = "—",
                validUntilUnix = 0L,
            )
            persist(ctx, account)
            Result.success(account)
        }

    /**
     * Mock of POST /account/redeem. Any non-empty code activates a 30-day
     * "Pro" subscription on the current account.
     */
    suspend fun redeem(ctx: Context, code: String): Result<Account> =
        withContext(Dispatchers.IO) {
            delay(700 + Random.nextLong(300))
            val current = load(ctx)
                ?: return@withContext Result.failure(Exception("Сначала войдите в аккаунт"))
            if (code.trim().isEmpty()) {
                return@withContext Result.failure(Exception("Введите код"))
            }
            val updated = current.copy(
                plan = "Pro",
                validUntilUnix = System.currentTimeMillis() / 1000 + 30L * 24 * 3600,
            )
            persist(ctx, updated)
            Result.success(updated)
        }

    fun logout(ctx: Context) {
        prefs(ctx).edit().clear().apply()
    }

    private fun persist(ctx: Context, a: Account) {
        prefs(ctx).edit()
            .putString(K_EMAIL, a.email)
            .putString(K_TOKEN, a.token)
            .putString(K_PLAN, a.plan)
            .putLong(K_UNTIL, a.validUntilUnix)
            .apply()
    }
}
