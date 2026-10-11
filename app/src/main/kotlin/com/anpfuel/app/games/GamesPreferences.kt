package com.anpfuel.app.games

import android.content.Context
import android.content.SharedPreferences
import com.anpfuel.domain.games.ArcadeRules
import com.anpfuel.domain.games.GameId
import com.anpfuel.domain.games.GamePhase
import com.anpfuel.domain.games.LocalBestPolicy

interface GameValues {
    fun readInt(key: String): Int
    fun writeInt(key: String, value: Int): Boolean
}

data class GameOptions(val muted: Boolean = true, val reducedMotion: Boolean = false, val lowPower: Boolean = false)
data class BestSave(val best: Int, val saved: Boolean)

/** Only two bounded integer records; no identities, traces, queues or unfinished games. */
class GamesRecords(private val values: GameValues) {
    fun best(game: GameId): Int = synchronized(lock) { LocalBestPolicy.sanitize(values.readInt(key(game))) }
    fun record(game: GameId, phase: GamePhase, score: Int): BestSave = synchronized(lock) {
        val previous = best(game)
        val next = LocalBestPolicy.update(previous, phase, score)
        if (next == previous) BestSave(previous, true)
        else if (values.writeInt(key(game), next)) BestSave(next, true)
        else BestSave(previous, false)
    }
    private fun key(game: GameId): String = "${game.name}_${ArcadeRules.VERSION}_best"
    private companion object { val lock = Any() }
}

class GamesPreferences(context: Context, storageName: String = "offline_games") {
    private val preferences = context.applicationContext.getSharedPreferences(storageName, Context.MODE_PRIVATE)
    val records = GamesRecords(PreferenceGameValues(preferences))
    fun options(): GameOptions = GameOptions(boolean("muted", true), boolean("reduced_motion", false), boolean("low_power", false))
    fun saveOptions(options: GameOptions) {
        preferences.edit().putBoolean("muted", options.muted)
            .putBoolean("reduced_motion", options.reducedMotion).putBoolean("low_power", options.lowPower).apply()
    }
    private fun boolean(key: String, default: Boolean): Boolean =
        try { preferences.getBoolean(key, default) } catch (_: ClassCastException) { default }
}

/** SharedPreferences changes its memory cache even when commit reports disk failure. */
internal class PreferenceGameValues(private val preferences: SharedPreferences) : GameValues {
    override fun readInt(key: String): Int = try { preferences.getInt(key, 0) } catch (_: ClassCastException) { 0 }
    override fun writeInt(key: String, value: Int): Boolean {
        val previous = readInt(key)
        val saved = preferences.edit().putInt(key, value).commit()
        if (!saved) {
            // Restore the cache too; retry must attempt a real write instead of
            // acknowledging the failed new value as an already saved record.
            preferences.edit().putInt(key, previous).apply()
        }
        return saved
    }
}
