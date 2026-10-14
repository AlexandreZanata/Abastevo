package com.anpfuel.data.remote

import io.mockk.*
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import org.json.JSONObject
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class SignedCommunityHelpfulnessTest {
 @Test fun `normal helpfulness uses signed owner scoped writes and query reads`()=runTest {
    val proof=mockk<PhotoProofTransport>()
    coEvery {proof.identityScope()} returns "account:synthetic:key"
    val body=slot<JSONObject>()
    coEvery {proof.post(any(),any(),capture(body),"account:synthetic:key")} returns JSONObject("""{"vote_id":"v1","helpful":1,"not_helpful":0,"total":1,"my_choice":"HELPFUL"}""")
    coEvery {proof.get(any(),"account:synthetic:key","decision_sequence=2")} returns JSONObject("""{"decision_sequence":2,"helpful":1,"not_helpful":0,"total":1,"my_choice":"HELPFUL"}""")
    val gateway=CommunityHelpfulnessHttpClient(OkHttpClient(),"https://example.invalid",proof)
    val receipt=gateway.castHelpfulness("123e4567-e89b-12d3-a456-426614174000","stable-command","HELPFUL",2)
    assertEquals(1L,receipt.helpful)
    assertEquals("stable-command",body.captured.getString("client_submission_id"))
    assertFalse(body.captured.has("location"))
    assertEquals("HELPFUL",gateway.tallyFor("123e4567-e89b-12d3-a456-426614174000",2).myChoice)
    coVerify(exactly=1) {proof.post("/v1/observations/123e4567-e89b-12d3-a456-426614174000/helpfulness","stable-command",any(),"account:synthetic:key")}
 }
 @Test fun `bound owner reaches transport without being replaced by the latest owner`()=runTest {
    val proof=mockk<PhotoProofTransport>()
    coEvery {proof.post(any(),any(),any(),"account:original:key")} returns JSONObject("""{"vote_id":"v1","helpful":1,"not_helpful":0,"total":1,"my_choice":"HELPFUL"}""")
    val gateway=CommunityHelpfulnessHttpClient(OkHttpClient(),"https://example.invalid",proof)
    gateway.castHelpfulness("123e4567-e89b-12d3-a456-426614174000","stable","HELPFUL",2,"account:original:key")
    coVerify(exactly=0){proof.identityScope()}
    coVerify(exactly=1){proof.post(any(),"stable",any(),"account:original:key")}
 }

}
