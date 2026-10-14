package com.anpfuel.application.usecase.community

import com.anpfuel.domain.community.CityCommunitySummaryReader
import com.anpfuel.domain.community.FeedCity

/** Manual canonical city lookup and anonymous summary; independent of ANP cache/account/GPS. */
class GetCityCommunitySummaryUseCase(
    private val reader: CityCommunitySummaryReader,
    private val cityFeed: GetCityCommunityFeedUseCase,
) {
    suspend fun city(): FeedCity? = cityFeed.city()
    suspend operator fun invoke(city: FeedCity) = reader.readSummary(city)
}
