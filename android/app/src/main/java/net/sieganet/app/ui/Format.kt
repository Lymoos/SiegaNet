package net.sieganet.app.ui

fun formatBytes(n: Long): String {
    if (n < 1024) return "$n Б"
    val units = listOf("КБ", "МБ", "ГБ", "ТБ")
    var v = n / 1024.0
    var i = 0
    while (v >= 1024 && i < units.lastIndex) {
        v /= 1024; i++
    }
    val num = if (v >= 100) v.toLong().toString() else String.format("%.1f", v)
    return "$num ${units[i]}"
}

fun formatDuration(sinceUnix: Long, nowSec: Long): String {
    val total = (nowSec - sinceUnix).coerceAtLeast(0)
    val h = total / 3600
    val m = (total % 3600) / 60
    val s = total % 60
    return "%d:%02d:%02d".format(h, m, s)
}
