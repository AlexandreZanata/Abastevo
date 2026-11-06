package com.anpfuel.app.ui.forum

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ForumStringsTest {

    private val requiredKeys = listOf(
        "forum_card_title",
        "forum_card_description",
        "forum_card_cta",
        "forum_screen_title",
        "forum_detail_title",
        "forum_category_all",
        "forum_kind_editorial",
        "forum_kind_news",
        "forum_kind_topic",
        "forum_kind_bulletin",
        "forum_feed_empty_title",
        "forum_feed_empty_filtered_hint",
        "forum_filters",
        "forum_filters_title",
        "forum_filter_clear",
        "forum_filter_all_ufs",
        "forum_filter_ibge",
        "forum_filter_invalid_ibge",
        "forum_filter_all_fuels",
        "forum_filter_station",
        "forum_filter_invalid_station",
        "forum_filter_apply",
        "forum_filter_cancel",
        "forum_load_more",
        "forum_retry",
        "forum_offline_notice",
        "forum_unavailable_title",
        "forum_unavailable_hint",
        "forum_post_not_found_title",
        "forum_post_not_found_hint",
        "forum_read_source",
        "forum_external_link_dialog_title",
        "forum_external_link_dialog_message",
        "forum_external_link_dialog_confirm",
        "forum_external_link_dialog_cancel",
        "forum_external_link_invalid",
        "forum_updated_label",
        "forum_social_disabled_notice",
        "forum_comments_disabled",
        "forum_comments_title",
        "forum_comments_empty",
        "forum_reply_button",
        "forum_replying_to",
        "forum_cancel_reply",
        "forum_replies_count",
        "forum_hide_replies",
        "forum_comment_composer_hint",
        "forum_comment_char_counter",
        "forum_comment_publish",
        "forum_comment_submitting",
        "forum_comment_pending_receipt",
        "forum_sign_in_to_participate",
        "forum_edit_comment",
        "forum_edit_dialog_title",
        "forum_edit_save",
        "forum_delete_comment",
        "forum_delete_dialog_title",
        "forum_delete_dialog_message",
        "forum_delete_confirm",
        "forum_report_button",
        "forum_report_dialog_title",
        "forum_report_dialog_reason_prompt",
        "forum_report_reason_spam",
        "forum_report_reason_offensive",
        "forum_report_reason_misinformation",
        "forum_report_reason_commercial",
        "forum_report_reason_legal_notice",
        "forum_report_reason_other",
        "forum_report_detail_hint",
        "forum_report_submit",
        "forum_report_success",
        "forum_conflict_error",
        "forum_rate_limited_error",
        "forum_new_topic",
        "forum_edit_topic",
        "forum_delete_topic",
        "forum_delete_topic_dialog_title",
        "forum_delete_topic_dialog_message",
        "forum_topic_title_label",
        "forum_topic_title_hint",
        "forum_topic_title_counter",
        "forum_topic_body_label",
        "forum_topic_body_hint",
        "forum_topic_body_counter",
        "forum_topic_category_label",
        "forum_topic_category_select_prompt",
        "forum_topic_region_label",
        "forum_topic_municipality_label",
        "forum_topic_fuel_label",
        "forum_topic_alias_label",
        "forum_topic_alias_hint",
        "forum_topic_publish",
        "forum_topic_save_changes",
        "forum_topic_submitting",
        "forum_topic_pending_receipt",
        "forum_topic_discard_dialog_title",
        "forum_topic_discard_dialog_message",
        "forum_topic_discard_confirm",
        "forum_topic_discard_cancel",
        "forum_topic_not_found",
        "forum_topic_validation_title_empty",
        "forum_topic_validation_title_too_long",
        "forum_topic_validation_body_empty",
        "forum_topic_validation_body_too_long",
        "forum_topic_validation_category",
    )

    private val localeDirs = listOf(
        "values",
        "values-pt-rBR",
        "values-es",
        "values-de",
        "values-fr",
        "values-ru",
        "values-ja",
        "values-zh-rCN",
    )

    @Test
    fun `all forum strings are defined in all supported locales`() {
        val resDir = File("src/main/res")
        val factory = DocumentBuilderFactory.newInstance()
        val builder = factory.newDocumentBuilder()

        for (dirName in localeDirs) {
            val stringsFile = File(resDir, "$dirName/strings.xml")
            assertTrue(stringsFile.exists(), "strings.xml must exist for locale $dirName")

            val doc = builder.parse(stringsFile)
            val stringNodes = doc.getElementsByTagName("string")
            val foundKeys = mutableSetOf<String>()

            for (i in 0 until stringNodes.length) {
                val node = stringNodes.item(i)
                val nameAttr = node.attributes.getNamedItem("name")?.nodeValue
                if (nameAttr != null) {
                    foundKeys.add(nameAttr)
                }
            }

            for (key in requiredKeys) {
                assertTrue(
                    foundKeys.contains(key),
                    "Key '$key' missing in $dirName/strings.xml",
                )
            }
        }
    }
}
