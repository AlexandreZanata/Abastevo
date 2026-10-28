package com.anpfuel.domain.repository

import com.anpfuel.domain.contributor.ContributorPage

/** CTB-10 public contributors read with opaque cursor paging. */
interface ContributorsGateway {
    suspend fun page(cursor: String?): ContributorPage
}
