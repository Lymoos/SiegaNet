package net.sieganet.app.api

import kotlin.math.roundToInt
import kotlin.random.Random

/**
 * Mock of GET /servers — same seed list as the desktop mock so both clients
 * demo identically. Real source of the list is decided at core-integration
 * time (per-config single server today; core-provided list later).
 */
object MockServers {

    private data class Seed(
        val id: String, val country: String, val city: String,
        val host: String, val basePing: Int, val baseLoad: Int,
    )

    private val seeds = listOf(
        Seed("nl-ams-1", "Нидерланды", "Амстердам", "ams1.siega.net", 46, 34),
        Seed("de-fra-1", "Германия", "Франкфурт", "fra1.siega.net", 51, 58),
        Seed("fi-hel-1", "Финляндия", "Хельсинки", "hel1.siega.net", 28, 22),
        Seed("se-sto-1", "Швеция", "Стокгольм", "sto1.siega.net", 37, 41),
        Seed("gb-lon-1", "Великобритания", "Лондон", "lon1.siega.net", 62, 66),
        Seed("fr-par-1", "Франция", "Париж", "par1.siega.net", 59, 48),
        Seed("pl-waw-1", "Польша", "Варшава", "waw1.siega.net", 33, 29),
        Seed("tr-ist-1", "Турция", "Стамбул", "ist1.siega.net", 74, 71),
        Seed("kz-ala-1", "Казахстан", "Алматы", "ala1.siega.net", 88, 18),
        Seed("ae-dxb-1", "ОАЭ", "Дубай", "dxb1.siega.net", 112, 52),
        Seed("sg-sin-1", "Сингапур", "Сингапур", "sin1.siega.net", 168, 44),
        Seed("jp-tyo-1", "Япония", "Токио", "tyo1.siega.net", 196, 37),
        Seed("us-nyc-1", "США", "Нью-Йорк", "nyc1.siega.net", 128, 63),
        Seed("us-lax-1", "США", "Лос-Анджелес", "lax1.siega.net", 182, 49),
        Seed("br-sao-1", "Бразилия", "Сан-Паулу", "sao1.siega.net", 224, 26),
    )

    fun list(): List<Server> = seeds.map {
        Server(
            id = it.id,
            country = it.country,
            city = it.city,
            host = it.host,
            pingMs = (it.basePing * (1 + Random.nextDouble(-0.12, 0.12))).roundToInt(),
            loadPct = (it.baseLoad + Random.nextInt(-4, 5)).coerceIn(5, 95),
        )
    }

    fun byId(id: String?): Server? = list().firstOrNull { it.id == id }
}
