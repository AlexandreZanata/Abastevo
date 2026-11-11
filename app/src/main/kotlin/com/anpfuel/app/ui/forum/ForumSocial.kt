package com.anpfuel.app.ui.forum

import android.content.Intent
import android.text.format.DateUtils
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Verified
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.anpfuel.app.R
import com.anpfuel.domain.forum.ForumPostKind
import java.time.Instant

/**
 * Social-feed building blocks of the forum, in the spirit of X: round avatar,
 * author line with a verified mark for official Abastevo posts, relative
 * time and a row of icon actions. Posts sit edge to edge on the white screen
 * with hairlines between them instead of cards.
 */

/** Author shown for a post or comment: its public alias, or the official account. */
internal data class ForumAuthor(val name: String, val official: Boolean)

/** Editorial, news and bulletins are written by Abastevo itself. */
internal fun ForumPostKind.isOfficial(): Boolean = when (this) {
    ForumPostKind.EDITORIAL, ForumPostKind.NEWS, ForumPostKind.BULLETIN, ForumPostKind.UNKNOWN -> true
    ForumPostKind.COMMUNITY, ForumPostKind.TOPIC -> false
}

@Composable
internal fun forumPostAuthor(kind: ForumPostKind, publicAlias: String?): ForumAuthor = when {
    kind.isOfficial() -> ForumAuthor(stringResource(R.string.app_name).replaceFirstChar { it.titlecase() }, official = true)
    !publicAlias.isNullOrBlank() -> ForumAuthor(publicAlias, official = false)
    else -> ForumAuthor(stringResource(R.string.forum_author_driver), official = false)
}

@Composable
internal fun forumCommentAuthor(publicAlias: String?): ForumAuthor =
    ForumAuthor(publicAlias?.takeIf { it.isNotBlank() } ?: stringResource(R.string.forum_author_driver), official = false)

/** Round avatar: the brand mark for official posts, initials on black for people. */
@Composable
internal fun ForumAvatar(author: ForumAuthor, size: Dp = 44.dp, modifier: Modifier = Modifier) {
    val scheme = MaterialTheme.colorScheme
    Surface(
        shape = CircleShape,
        color = scheme.primary,
        contentColor = scheme.onPrimary,
        modifier = modifier.size(size),
    ) {
        Box(contentAlignment = Alignment.Center) {
            if (author.official) {
                Image(
                    painter = painterResource(R.drawable.ic_abastevo_logo),
                    contentDescription = null,
                    modifier = Modifier.size(size * 0.6f),
                )
            } else {
                Text(
                    text = author.initials(),
                    style = if (size >= 40.dp) MaterialTheme.typography.titleMedium else MaterialTheme.typography.labelLarge,
                    fontWeight = FontWeight.Bold,
                )
            }
        }
    }
}

private fun ForumAuthor.initials(): String =
    name.split(' ', '_', '-', '.')
        .filter { it.isNotBlank() }
        .take(2)
        .joinToString("") { it.first().uppercase() }
        .ifEmpty { "?" }

/** "Name ✓ · 2 h" in one line; the name keeps priority over the time. */
@Composable
internal fun ForumAuthorLine(author: ForumAuthor, timeIso: String, modifier: Modifier = Modifier) {
    val scheme = MaterialTheme.colorScheme
    Row(
        modifier = modifier,
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Text(
            text = author.name,
            style = MaterialTheme.typography.titleSmall,
            fontWeight = FontWeight.Bold,
            color = scheme.onSurface,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f, fill = false),
        )
        if (author.official) {
            Icon(
                imageVector = Icons.Filled.Verified,
                contentDescription = stringResource(R.string.forum_official_account),
                tint = scheme.onSurface,
                modifier = Modifier.size(16.dp),
            )
        }
        Text(
            text = "· ${forumRelativeTime(timeIso)}",
            style = MaterialTheme.typography.bodySmall,
            color = scheme.onSurfaceVariant,
            maxLines = 1,
        )
    }
}

/** Short relative time ("2 h", "3 d") that falls back to the date for older posts. */
@Composable
internal fun forumRelativeTime(isoString: String): String {
    // Recompose on locale changes so the abbreviation follows the app language.
    LocalConfiguration.current
    return try {
        val millis = Instant.parse(isoString).toEpochMilli()
        val now = System.currentTimeMillis()
        if (now - millis < DateUtils.WEEK_IN_MILLIS) {
            DateUtils.getRelativeTimeSpanString(
                millis,
                now,
                DateUtils.MINUTE_IN_MILLIS,
                DateUtils.FORMAT_ABBREV_RELATIVE,
            ).toString()
        } else {
            formatForumDate(isoString)
        }
    } catch (_: Exception) {
        formatForumDate(isoString)
    }
}

/** One icon action of the post/comment action row, with an optional count. */
@Composable
internal fun ForumIconAction(
    icon: ImageVector,
    contentDescription: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    count: Int? = null,
) {
    val tint = MaterialTheme.colorScheme.onSurfaceVariant
    Row(modifier = modifier, verticalAlignment = Alignment.CenterVertically) {
        IconButton(onClick = onClick, modifier = Modifier.size(40.dp)) {
            Icon(icon, contentDescription = contentDescription, tint = tint, modifier = Modifier.size(19.dp))
        }
        if (count != null && count > 0) {
            Text(
                text = count.toString(),
                style = MaterialTheme.typography.labelMedium,
                color = tint,
                modifier = Modifier.padding(end = 8.dp),
            )
        }
    }
}

/** Shares a post through the system sheet: title and excerpt as plain text. */
@Composable
internal fun rememberForumShare(): (title: String, excerpt: String) -> Unit {
    val context = LocalContext.current
    return { title, excerpt ->
        val text = listOf(title, excerpt).filter { it.isNotBlank() }.joinToString("\n\n")
        val send = Intent(Intent.ACTION_SEND).apply {
            type = "text/plain"
            putExtra(Intent.EXTRA_TEXT, text)
        }
        context.startActivity(Intent.createChooser(send, null))
    }
}

/** One block of a plain-text post body. */
internal sealed interface ForumBodyBlock {
    data class Heading(val text: String) : ForumBodyBlock
    data class Paragraph(val text: String) : ForumBodyBlock
    data class Bullets(val items: List<String>) : ForumBodyBlock
}

private const val HEADING_MAX_CHARS = 80

/**
 * Splits a plain-text body on blank lines. A short single line without final
 * punctuation reads as a subheading, and "- " lines form a bullet list; the
 * text itself is never changed.
 */
internal fun parseForumBody(body: String): List<ForumBodyBlock> =
    body.replace("\r\n", "\n")
        .split(Regex("\n\\s*\n"))
        .map { it.trim() }
        .filter { it.isNotEmpty() }
        .map { block ->
            val lines = block.lines().map { it.trim() }.filter { it.isNotEmpty() }
            when {
                lines.all { it.startsWith("- ") } -> ForumBodyBlock.Bullets(lines.map { it.removePrefix("- ").trim() })
                lines.size == 1 && block.length <= HEADING_MAX_CHARS && block.last() !in ".:;,!?…" ->
                    ForumBodyBlock.Heading(block)
                else -> ForumBodyBlock.Paragraph(block)
            }
        }

@Composable
internal fun ForumBodyText(body: String, modifier: Modifier = Modifier) {
    val scheme = MaterialTheme.colorScheme
    androidx.compose.foundation.layout.Column(
        modifier = modifier,
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        parseForumBody(body).forEach { block ->
            when (block) {
                is ForumBodyBlock.Heading -> Text(
                    text = block.text,
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.Bold,
                    color = scheme.onSurface,
                    modifier = Modifier.padding(top = 8.dp),
                )
                is ForumBodyBlock.Paragraph -> Text(
                    text = block.text,
                    style = MaterialTheme.typography.bodyLarge,
                    color = scheme.onSurface,
                )
                is ForumBodyBlock.Bullets -> androidx.compose.foundation.layout.Column(
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    block.items.forEach { item ->
                        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                            Text("•", style = MaterialTheme.typography.bodyLarge, color = scheme.onSurface)
                            Text(item, style = MaterialTheme.typography.bodyLarge, color = scheme.onSurface)
                        }
                    }
                }
            }
        }
    }
}
