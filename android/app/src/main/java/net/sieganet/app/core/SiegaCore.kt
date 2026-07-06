package net.sieganet.app.core

/**
 * THE integration point with the Go core.
 *
 * The real implementation is a gomobile-bound module (`.aar` built from the
 * repo's `mobile/` bind layer): primitive-typed methods only, exactly this
 * shape. The UI, the VpnService and the repository already program against
 * this interface, so wiring the real core is a drop-in:
 *
 *   1. gomobile bind -target=android -o app/libs/siega.aar ./mobile
 *   2. uncomment `implementation(files("libs/siega.aar"))` in app/build.gradle.kts
 *   3. replace [CoreRegistry.core] with the gomobile-backed implementation:
 *
 *      object GomobileSiegaCore : SiegaCore {
 *          override fun start(tunFd: Int, configJson: String) = mobile.Mobile.start(tunFd, configJson)
 *          override fun stop() = mobile.Mobile.stop()
 *          override fun statusJson(): String = mobile.Mobile.statusJson()
 *      }
 *
 * No VPN logic lives on the Kotlin side: the service owns the TUN fd and the
 * lifecycle, the core owns the tunnel (QUIC, auth, crypto, data-plane).
 */
interface SiegaCore {
    /**
     * Start the tunnel on an established TUN device.
     *
     * @param tunFd      detached file descriptor of the TUN device
     *                   (ParcelFileDescriptor.detachFd()); the core owns it
     *                   until [stop]
     * @param configJson peer config (server host, peer_id, psk, tunnel_path…)
     *                   as passed by the config import
     */
    fun start(tunFd: Int, configJson: String)

    /** Tear the tunnel down and release the fd. Idempotent. */
    fun stop()

    /** Status document identical to the desktop control API's GET /status. */
    fun statusJson(): String
}

/** Swap point between the mock and the gomobile core. */
object CoreRegistry {
    val core: SiegaCore = MockSiegaCore()
}
