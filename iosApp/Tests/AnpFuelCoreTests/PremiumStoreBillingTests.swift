import XCTest
@testable import AnpFuelCore

/// B-BR-STORE-04/08/10: wire decoding, purchase availability and the
/// StoreKit finish rule. The JSON mirrors the Go status handler.
final class PremiumStoreBillingTests: XCTestCase {
    private func status(_ json: String) throws -> PremiumStatusWire {
        return try PremiumStoreBilling.decoder().decode(PremiumStatusWire.self, from: Data(json.utf8))
    }

    private let freeWithOffer = """
    {"policy_version":1,"state":"NONE","reason":"no-purchase","capabilities":[],"valid_until":null,
     "retained_data_access":true,"payments_configured":true,"tiers":[],
     "management_url":"https://play.google.com/store/account/subscriptions",
     "store_offers":[
       {"provider":"play","products":[{"product_id":"abastevo_premium","plan_ids":["monthly"]}],
        "account_binding":"\(String(repeating: "a", count: 64))","management_url":"https://play.google.com/store/account/subscriptions?package=com.anpfuel.app"},
       {"provider":"app_store","products":[{"product_id":"com.anpfuel.app.premium.monthly","plan_ids":[]},
        {"product_id":"com.anpfuel.app.premium.yearly","plan_ids":[]}],
        "account_binding":"6f1c2b8e-6d2a-8c3e-9a41-2b7d5e0c1f34","management_url":"https://apps.apple.com/account/subscriptions"}]}
    """

    func testDecodesStatusWithStoreOffers() throws {
        let decoded = try status(freeWithOffer)
        XCTAssertEqual(decoded.storeOffers.count, 2)
        XCTAssertEqual(decoded.appStoreOffer?.products.map(\.productId), ["com.anpfuel.app.premium.monthly", "com.anpfuel.app.premium.yearly"])
        XCTAssertFalse(decoded.isPremiumActive)
    }

    func testOlderServerWithoutOffersDecodesAsUnavailable() throws {
        let decoded = try status("""
        {"policy_version":1,"state":"NONE","reason":"payments-disabled","capabilities":[],"valid_until":null,
         "retained_data_access":true,"payments_configured":false,"management_url":"https://play.google.com/store/account/subscriptions"}
        """)
        XCTAssertEqual(decoded.storeOffers, [])
        XCTAssertEqual(PremiumStoreBilling.availability(for: decoded), .unavailable)
    }

    func testAvailabilityCarriesTheServerBindingAsAppAccountToken() throws {
        let decoded = try status(freeWithOffer)
        guard case .available(let ids, let token) = PremiumStoreBilling.availability(for: decoded) else {
            return XCTFail("App Store offer not available")
        }
        XCTAssertEqual(ids.count, 2)
        XCTAssertEqual(token.uuidString.lowercased(), "6f1c2b8e-6d2a-8c3e-9a41-2b7d5e0c1f34")
    }

    func testActivePremiumFromAnySourceHidesPurchase() throws {
        let decoded = try status("""
        {"policy_version":1,"state":"ACTIVE","reason":"verified-web","capabilities":["cloud.sync"],
         "valid_until":"2026-12-23T12:00:00Z","retained_data_access":true,"payments_configured":true,
         "tiers":[{"tier":"PRO","source":"web","valid_until":"2026-12-23T12:00:00Z"}],
         "store_offers":[{"provider":"app_store","products":[{"product_id":"p","plan_ids":[]}],
          "account_binding":"6f1c2b8e-6d2a-8c3e-9a41-2b7d5e0c1f34","management_url":"https://apps.apple.com/account/subscriptions"}]}
        """)
        XCTAssertTrue(decoded.isPremiumActive)
        XCTAssertEqual(decoded.tiers.first?.source, "web")
        XCTAssertEqual(PremiumStoreBilling.availability(for: decoded), .active)
    }

    func testMalformedBindingNeverStartsAPurchase() throws {
        let json = freeWithOffer.replacingOccurrences(of: "6f1c2b8e-6d2a-8c3e-9a41-2b7d5e0c1f34", with: "not-a-uuid")
        XCTAssertEqual(PremiumStoreBilling.availability(for: try status(json)), .unavailable)
    }

    func testFinishOnlyAfterAcceptanceOrDefinitiveRefusal() throws {
        let receipt = try PremiumStoreBilling.decoder().decode(PremiumReceiptWire.self, from: Data("""
        {"operation_id":"55555555-5555-4555-8555-555555555555","replayed":false,
         "status":{"policy_version":1,"state":"ACTIVE","reason":"verified","capabilities":["cloud.sync"],
          "valid_until":"2026-12-23T12:00:00Z","retained_data_access":true,"payments_configured":true,
          "tiers":[{"tier":"PRO","source":"app_store","valid_until":"2026-12-23T12:00:00Z"}],"store_offers":[]}}
        """.utf8))
        XCTAssertEqual(PremiumStoreBilling.finishDecision(for: .success(receipt)), .finish)
        XCTAssertEqual(PremiumStoreBilling.finishDecision(for: .failure(.denied)), .finish)
        XCTAssertEqual(PremiumStoreBilling.finishDecision(for: .failure(.invalid)), .finish)
        for retryable in [PremiumApiError.unavailable, .retry, .quota, .authRequired] {
            XCTAssertEqual(PremiumStoreBilling.finishDecision(for: .failure(retryable)), .keepForRetry)
        }
    }

    func testHttpErrorMapping() {
        XCTAssertEqual(PremiumStoreBilling.error(httpStatus: 400, code: "subscription.invalid"), .invalid)
        XCTAssertEqual(PremiumStoreBilling.error(httpStatus: 401, code: nil), .authRequired)
        XCTAssertEqual(PremiumStoreBilling.error(httpStatus: 403, code: "subscription.proof-denied"), .denied)
        XCTAssertEqual(PremiumStoreBilling.error(httpStatus: 409, code: "subscription.retry-verification"), .retry)
        XCTAssertEqual(PremiumStoreBilling.error(httpStatus: 429, code: "subscription.quota"), .quota)
        XCTAssertEqual(PremiumStoreBilling.error(httpStatus: 503, code: "subscription.unavailable"), .unavailable)
    }

    func testSignedTransportFailuresKeepRetryOrSignInMeaning() {
        XCTAssertEqual(PremiumStoreBilling.error(signedTransport: .refused(status: 403, code: "subscription.proof-denied", retryAfterSeconds: nil)), .denied)
        XCTAssertEqual(PremiumStoreBilling.error(signedTransport: .refused(status: 503, code: nil, retryAfterSeconds: nil)), .unavailable)
        XCTAssertEqual(PremiumStoreBilling.error(signedTransport: .sessionRequired), .authRequired)
        XCTAssertEqual(PremiumStoreBilling.error(signedTransport: .identityUnavailable), .authRequired)
        XCTAssertEqual(PremiumStoreBilling.error(signedTransport: .offline), .unavailable)
    }

    func testPurchaseBodyIsTheSignedAppStoreCommand() throws {
        let body = try PremiumStoreBilling.purchaseBody(
            familyId: "11111111-1111-4111-8111-111111111111",
            accessToken: "synthetic-access",
            operationId: "55555555-5555-4555-8555-55555555555A",
            originalTransactionId: 2_000_000_123_456_789
        )
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: String])
        XCTAssertEqual(json["provider"], "app_store")
        XCTAssertEqual(json["purchase_token"], "2000000123456789")
        XCTAssertEqual(json["operation_id"], "55555555-5555-4555-8555-55555555555a")
        XCTAssertEqual(PremiumStoreBilling.purchasePath(.restore), "/v1/premium/purchases/restore")
    }
}
