import XCTest
@testable import AnpFuelCore

/// Company access, device enrollment, reference feed and attachment session wire rules.
/// Reference-feed and attachment rows are copied verbatim from `contracts/testdata/enterprise/sync`;
/// the company-plane shapes follow the Go handlers (`companysession`, `organization` contexts,
/// `membership` redemption) and Android `CompanyAccessClient`.
final class EnterpriseAccessWireTests: XCTestCase {
    private let account = "7b0e2a54-0000-4000-8000-000000000001"
    private let org = "7b0e2a54-0000-4000-8000-000000000065"
    private let member = "7b0e2a54-0000-4000-8000-0000000000c9"

    private func json(_ text: String) throws -> [String: Any] {
        return try XCTUnwrap(try JSONSerialization.jsonObject(with: Data(text.utf8)) as? [String: Any])
    }

    // MARK: - Company cookie plane

    func testSessionCookieMustBeTheSingleHostOnlyStrictCookie() {
        let secret = String(repeating: "a", count: 64)
        let good = "__Host-empresas-session=abv_company_\(secret); Path=/; Expires=Thu, 19 Nov 2026 20:00:00 GMT; HttpOnly; Secure; SameSite=Strict"
        XCTAssertEqual(EnterpriseCompanyWire.sessionCookie(fromSetCookie: good), "__Host-empresas-session=abv_company_\(secret)")
        XCTAssertNil(EnterpriseCompanyWire.sessionCookie(fromSetCookie: nil))
        XCTAssertNil(EnterpriseCompanyWire.sessionCookie(fromSetCookie: good.replacingOccurrences(of: "; SameSite=Strict", with: "; SameSite=Lax")))
        XCTAssertNil(EnterpriseCompanyWire.sessionCookie(fromSetCookie: good.replacingOccurrences(of: "; HttpOnly", with: "")))
        XCTAssertNil(EnterpriseCompanyWire.sessionCookie(fromSetCookie: good.replacingOccurrences(of: "; Secure", with: "")))
        XCTAssertNil(EnterpriseCompanyWire.sessionCookie(fromSetCookie: good + "; Domain=abastevo.com.br"))
        XCTAssertNil(EnterpriseCompanyWire.sessionCookie(fromSetCookie: good + "; Max-Age=0"))
        XCTAssertNil(EnterpriseCompanyWire.sessionCookie(fromSetCookie: good + ", " + good))
        XCTAssertNil(EnterpriseCompanyWire.sessionCookie(fromSetCookie: good.replacingOccurrences(of: secret, with: String(repeating: "A", count: 64))))
        XCTAssertNil(EnterpriseCompanyWire.sessionCookie(fromSetCookie: "__Host-empresas-session=abv_company_csrf_\(secret); Path=/; HttpOnly; Secure; SameSite=Strict"))
    }

    func testLoginAnswerBindsTheProvingAccount() throws {
        let csrf = "abv_company_csrf_" + String(repeating: "b", count: 64)
        let body = try json(#"{"account_id":"\#(account)","csrf_token":"\#(csrf)","authenticated_at":"2026-11-15T12:00:00Z","expires_at":"2026-11-15T20:00:00Z"}"#)
        XCTAssertEqual(EnterpriseCompanyWire.loginCsrf(body, accountId: account), csrf)
        XCTAssertNil(EnterpriseCompanyWire.loginCsrf(body, accountId: org))
        let badCsrf = try json(#"{"account_id":"\#(account)","csrf_token":"abv_company_\#(String(repeating: "b", count: 64))","authenticated_at":"x","expires_at":"y"}"#)
        XCTAssertNil(EnterpriseCompanyWire.loginCsrf(badCsrf, accountId: account))
    }

    func testRefusalsMapToHonestStates() {
        XCTAssertEqual(EnterpriseCompanyWire.error(status: 401, code: "FRESH_AUTH_REQUIRED", path: "/v1/org-access/invitations/preview"), .freshAuthRequired)
        XCTAssertEqual(EnterpriseCompanyWire.error(status: 401, code: "UNAUTHENTICATED", path: "/v1/org-access/invitations/preview"), .signInRequired)
        XCTAssertEqual(EnterpriseCompanyWire.error(status: 404, code: "TOKEN_INVALID", path: "/v1/org-access/invitations/preview"), .invalidCode)
        XCTAssertEqual(EnterpriseCompanyWire.error(status: 409, code: "ALREADY_MEMBER", path: "/v1/org-access/invitations/accept"), .alreadyMember)
        XCTAssertEqual(EnterpriseCompanyWire.error(status: 403, code: "MANAGED_ACCOUNT_RESTRICTED", path: "/v1/org-access/invitations/accept"), .managedRestricted)
        XCTAssertEqual(EnterpriseCompanyWire.error(status: 429, code: "RATE_LIMITED", path: "/v1/org-access/invitations/preview"), .rateLimited)
        XCTAssertEqual(EnterpriseCompanyWire.error(status: 403, code: "ENTITLEMENT_REQUIRED", path: "/v1/orgs/\(org)/checklists/assets/\(member)/applicable"), .entitlementRequired)
        XCTAssertEqual(EnterpriseCompanyWire.error(status: 403, code: "ORG_SUSPENDED", path: "/v1/orgs/\(org)/checklists/assets/\(member)/applicable"), .organizationSuspended)
        // The account-context surface denies generically, whatever the body says.
        XCTAssertEqual(EnterpriseCompanyWire.error(status: 403, code: "ENTITLEMENT_REQUIRED", path: EnterpriseCompanyWire.contextsReadPath), .accessDenied)
        XCTAssertEqual(EnterpriseCompanyWire.error(status: 404, code: nil, path: EnterpriseCompanyWire.contextsListPath), .accessDenied)
        XCTAssertEqual(EnterpriseCompanyWire.error(status: 503, code: "UNAVAILABLE", path: EnterpriseCompanyWire.contextsListPath), .unavailable)

        XCTAssertEqual(EnterpriseSyncError.refused(status: 403, code: "DEVICE_REVOKED", retryAfterSeconds: nil).accessError, .deviceRevoked)
        XCTAssertEqual(EnterpriseSyncError.refused(status: 403, code: "MEMBERSHIP_SUSPENDED", retryAfterSeconds: nil).accessError, .membershipSuspended)
        XCTAssertEqual(EnterpriseSyncError.refused(status: 404, code: "NOT_FOUND", retryAfterSeconds: nil).accessError, .accessDenied)
        XCTAssertEqual(EnterpriseSyncError.offline.accessError, .offline)
        XCTAssertTrue(SignedRequestProfile.isEnterpriseCode("ENTITLEMENT_REQUIRED"))
        XCTAssertFalse(SignedRequestProfile.isEnterpriseCode("community.quota-exceeded"))
        XCTAssertEqual(SignedRequestProfile.enterpriseErrorCode(from: Data(#"{"error":{"code":"ORG_SUSPENDED"}}"#.utf8)), "ORG_SUSPENDED")
    }

    func testContextPagesAreBoundOrderedAndPaginated() throws {
        let first = "7b0e2a54-0000-4000-8000-000000000101"
        let second = "7b0e2a54-0000-4000-8000-000000000102"
        let page = try json(#"{"account_id":"\#(account)","organizations":[{"id":"\#(first)","name":"Transportes A","membership_id":"\#(member)","state":"ACTIVE","suspension_reason":null},{"id":"\#(second)","name":"Frota B","membership_id":"\#(member)","state":"SUSPENDED","suspension_reason":"BILLING"}],"next_after_org_id":""}"#)
        let parsed = try XCTUnwrap(EnterpriseCompanyWire.contextsPage(page, accountId: account, after: nil))
        XCTAssertEqual(parsed.contexts.map { $0.organizationId }, [first, second])
        XCTAssertEqual(parsed.contexts.last?.state, "SUSPENDED")
        XCTAssertNil(parsed.next)
        XCTAssertNil(EnterpriseCompanyWire.contextsPage(page, accountId: org, after: nil))
        XCTAssertNil(EnterpriseCompanyWire.contextsPage(page, accountId: account, after: second))
        let unordered = try json(#"{"account_id":"\#(account)","organizations":[{"id":"\#(second)","name":"B","membership_id":"\#(member)"},{"id":"\#(first)","name":"A","membership_id":"\#(member)"}],"next_after_org_id":""}"#)
        XCTAssertNil(EnterpriseCompanyWire.contextsPage(unordered, accountId: account, after: nil))

        let selected = parsed.contexts[0]
        let read = try json(#"{"account_id":"\#(account)","organization":{"id":"\#(first)","name":"Transportes A","membership_id":"\#(member)","state":"ACTIVE","suspension_reason":null},"employee_id":"7b0e2a54-0000-4000-8000-000000000131","permissions":["trip.own.start"],"invitation_grants":[],"invitation_validity_days":7}"#)
        let detail = try XCTUnwrap(EnterpriseCompanyWire.contextDetail(read, accountId: account, selected: selected))
        XCTAssertEqual(detail.employeeId, "7b0e2a54-0000-4000-8000-000000000131")
        XCTAssertNil(EnterpriseCompanyWire.contextDetail(read, accountId: account, selected: parsed.contexts[1]))
    }

    func testInvitationPreviewAndAcknowledgementFollowTheReviewedContract() throws {
        let digest = String(repeating: "c", count: 64)
        let preview = try json(#"{"organization":{"name":"Transportes A","legal_entity_verified":true},"grants":[{"role":"DRIVER","scope":"SELF"}],"confirmation_required":false,"consent_digest":"\#(digest)","notice":{"version":"enterprise-invitation-v1","text":"\#(EnterpriseCompanyWire.noticeText)"},"data_mode":"SYNTHETIC"}"#)
        let parsed = try XCTUnwrap(EnterpriseCompanyWire.invitationPreview(preview))
        XCTAssertEqual(parsed.grants, [EnterpriseInvitationGrant(role: "DRIVER", scope: "SELF")])
        XCTAssertTrue(parsed.legalEntityVerified)
        let changedNotice = try json(#"{"organization":{"name":"A","legal_entity_verified":true},"grants":[{"role":"DRIVER","scope":"SELF"}],"confirmation_required":false,"consent_digest":"\#(digest)","notice":{"version":"enterprise-invitation-v1","text":"other"},"data_mode":"SYNTHETIC"}"#)
        XCTAssertNil(EnterpriseCompanyWire.invitationPreview(changedNotice))
        let ownerSelf = try json(#"{"organization":{"name":"A","legal_entity_verified":1},"grants":[{"role":"OWNER","scope":"SELF"}],"confirmation_required":false,"consent_digest":"\#(digest)","notice":{"version":"enterprise-invitation-v1","text":"\#(EnterpriseCompanyWire.noticeText)"},"data_mode":"SYNTHETIC"}"#)
        XCTAssertNil(EnterpriseCompanyWire.invitationPreview(ownerSelf))

        let invitation = "7b0e2a54-0000-4000-8000-000000000201"
        let code = "AIN1.7b0e2a54000040008000000000000201.secret"
        XCTAssertEqual(EnterpriseCompanyWire.normalizeInvitationCode(" \(code) "), "AIN1.\(invitation).secret")
        let accepted = try json(#"{"organization_id":"\#(org)","invitation_id":"\#(invitation)","membership_id":"\#(member)","state":"ACCEPTED","membership_state":"INVITED","replayed":false,"data_mode":"SYNTHETIC","accepted_at":"2026-11-15T12:00:00Z","confirmation_deadline":"2026-11-22T12:00:00Z"}"#)
        let ack = try XCTUnwrap(EnterpriseCompanyWire.invitationAcknowledgement(accepted, code: code, accepted: true))
        XCTAssertTrue(ack.awaitsConfirmation)
        XCTAssertNil(EnterpriseCompanyWire.invitationAcknowledgement(accepted, code: "AIN1.\(org).secret", accepted: true))
        XCTAssertNil(EnterpriseCompanyWire.invitationAcknowledgement(accepted, code: code, accepted: false))
        let declined = try json(#"{"organization_id":"\#(org)","invitation_id":"\#(invitation)","membership_id":"\#(member)","state":"DECLINED","membership_state":"ENDED","replayed":true,"data_mode":"SYNTHETIC"}"#)
        XCTAssertTrue(try XCTUnwrap(EnterpriseCompanyWire.invitationAcknowledgement(declined, code: code, accepted: false)).isDeclined)
    }

    func testChecklistApplicableIsBoundToTheAskedAssetAndMoment() throws {
        let asset = "7b0e2a54-0000-4000-8000-000000009721"
        let template = "7b0e2a54-0000-4000-8000-000000009731"
        let required = try json(#"{"asset_id":"\#(asset)","context":"DEPARTURE","required":true,"match":"ASSET","evaluated_at":"2026-11-15T12:00:00Z","data_mode":"SYNTHETIC","template":{"template_id":"\#(template)","name":"Saída","version":2,"published_at":"2026-11-01T00:00:00Z","items":[{"position":1,"label":"Pneus","critical":false,"optional":true,"photo_on_not_ok":false},{"position":2,"label":"Freios","critical":true,"optional":false,"photo_on_not_ok":false}]},"policy":{"revision":3,"mode":"WARN","validity_seconds":43200,"effective_at":"2026-11-01T00:00:00Z"}}"#)
        let parsed = try XCTUnwrap(EnterpriseCompanyWire.checklistApplicable(required, assetId: asset, context: "DEPARTURE"))
        XCTAssertEqual(parsed.items.map { $0.label }, ["Pneus", "Freios"])
        XCTAssertEqual(parsed.version, 2)
        XCTAssertEqual(parsed.policyRevision, 3)
        XCTAssertNil(EnterpriseCompanyWire.checklistApplicable(required, assetId: template, context: "DEPARTURE"))
        XCTAssertNil(EnterpriseCompanyWire.checklistApplicable(required, assetId: asset, context: "RETURN"))
        let none = try json(#"{"asset_id":"\#(asset)","context":"DEPARTURE","required":false,"match":"NONE","evaluated_at":"2026-11-15T12:00:00Z","data_mode":"SYNTHETIC"}"#)
        XCTAssertEqual(EnterpriseCompanyWire.checklistApplicable(none, assetId: asset, context: "DEPARTURE")?.required, false)
    }

    // MARK: - Device enrollment

    func testDeviceContactVerdictsAreExactAndBound() throws {
        let partition = try XCTUnwrap(EnterprisePartition(accountId: account, origin: "https://api.example.invalid", organizationId: org))
        let enrollment = EnterpriseEnrollment(membershipId: member, enrollmentId: "7b0e2a54-0000-4000-8000-00000000012d", revision: 1)
        let family = "7b0e2a54-0000-4000-8000-000000000301"
        let request = "7b0e2a54-0000-4000-8000-000000000302"
        let common = #""schema_version":1,"request_id":"\#(request)","account_id":"\#(account)","api_origin":"https://api.example.invalid","org_id":"\#(org)","membership_id":"\#(member)","enrollment_id":"7b0e2a54-0000-4000-8000-00000000012d","family_id":"\#(family)","enrollment_revision":"2","server_time":"2026-11-15T12:00:00Z""#
        func contact(_ tail: String) throws -> EnterpriseContactOutcome {
            let fields = try EnterpriseDeviceWire.object(Data(("{" + common + tail + "}").utf8))
            return try EnterpriseDeviceWire.contact(fields, requestId: request, partition: partition, enrollment: enrollment, familyId: family)
        }
        let active = try contact(#","verdict":"ACTIVE","access_model":"EXISTING_ACCOUNT","offline_validity_seconds":2592000,"managed_unlock_seconds":null"#)
        XCTAssertEqual(active.verdict, .active)
        XCTAssertEqual(active.enrollment.revision, 2)
        XCTAssertEqual(try contact(#","verdict":"LOCK","revocation_class":"ACCESS_ENDED""#).verdict, .lock)
        XCTAssertEqual(try contact(#","verdict":"ERASE","revocation_class":"LOST_OR_COMPROMISED""#).verdict, .erase)
        XCTAssertThrowsError(try contact(#","verdict":"ERASE","revocation_class":"ACCESS_ENDED""#))
        XCTAssertThrowsError(try contact(#","verdict":"ACTIVE","access_model":"EXISTING_ACCOUNT","offline_validity_seconds":86400,"managed_unlock_seconds":null"#))
        XCTAssertThrowsError(try EnterpriseDeviceWire.contact(
            try EnterpriseDeviceWire.object(Data(("{" + common + #","verdict":"LOCK","revocation_class":"ACCESS_ENDED"}"#).utf8)),
            requestId: request, partition: partition, enrollment: enrollment, familyId: request
        ))

        let device = try EnterpriseDeviceWire.object(Data(#"{"id":"7b0e2a54-0000-4000-8000-00000000012d","membership_id":"\#(member)","label":"iOS","state":"ENROLLED","revision":1,"revocation_class":"","enrolled_at":"2026-11-15T12:00:00Z","last_sync_at":null,"revoked_at":null,"data_mode":"SYNTHETIC"}"#.utf8))
        XCTAssertEqual(try EnterpriseDeviceWire.device(device, membershipId: member).enrollmentId, "7b0e2a54-0000-4000-8000-00000000012d")
        XCTAssertThrowsError(try EnterpriseDeviceWire.device(device, membershipId: org))
    }

    // MARK: - Reference feed

    private let feedMember = "7b0e2a54-0000-4000-8000-000000008611"
    private let feedEnrollment = "7b0e2a54-0000-4000-8000-000000008613"
    private let feedRequest = "7b0e2a54-0000-4000-8000-000000008620"
    private let heldCursor = "eyJ2IjoxLCJyIjozfQ.c3ludGhldGljLXNpZ25hdHVyZQ"

    func testReferenceRequestsMatchTheSharedCanonicalBytes() throws {
        let noCursor = try EnterpriseReferenceWire.requestBody(
            requestId: feedRequest, familyId: "7b0e2a54-0000-4000-8000-000000008621", accessToken: "synthetic-reference-access",
            membershipId: feedMember, enrollmentId: feedEnrollment, cursor: nil
        )
        XCTAssertEqual(String(decoding: noCursor, as: UTF8.self), Vectors.request_no_cursor)
        let withCursor = try EnterpriseReferenceWire.requestBody(
            requestId: feedRequest, familyId: "7b0e2a54-0000-4000-8000-000000008621", accessToken: "synthetic-reference-access",
            membershipId: feedMember, enrollmentId: feedEnrollment, cursor: heldCursor
        )
        XCTAssertEqual(String(decoding: withCursor, as: UTF8.self), Vectors.request_with_cursor)
    }

    func testReferenceResponsesApplyWholeOrBindToNothing() throws {
        let reset = try EnterpriseReferenceWire.parseResponse(
            Data(Vectors.response_reset_new_cursor_one_asset.utf8), requestId: feedRequest, membershipId: feedMember,
            enrollmentId: feedEnrollment, sentCursor: nil, heldRevision: nil
        )
        let cache = try EnterpriseReferenceWire.apply(prior: nil, response: reset, receivedAtMillis: 1_000)
        XCTAssertEqual(cache.assets.map { $0.name }, ["Caminhão 12"])
        XCTAssertEqual(cache.assets.first?.meters.first?.kind, .odometer)
        XCTAssertTrue(cache.isConfirmed(nowMillis: 1_000 + 3_600_000))
        XCTAssertFalse(cache.isConfirmed(nowMillis: 1_000 + EnterpriseReferenceCache.confirmationMillis))

        let held = EnterpriseReferenceCache(membershipId: feedMember, enrollmentId: feedEnrollment, revision: 3, cursor: heldCursor,
                                            confirmedAt: reset.confirmedAt, forbidden: false, assets: cache.assets, receivedAtMillis: 1_000)
        let delta = try EnterpriseReferenceWire.parseResponse(
            Data(Vectors.response_delta_delete_and_upsert.utf8), requestId: feedRequest, membershipId: feedMember,
            enrollmentId: feedEnrollment, sentCursor: heldCursor, heldRevision: 3
        )
        let next = try EnterpriseReferenceWire.apply(prior: held, response: delta, receivedAtMillis: 2_000)
        XCTAssertEqual(next.assets.map { $0.name }, ["Escavadeira Ç 😀"])
        XCTAssertEqual(next.assets.first?.meters.first?.modulus, 100_000)
        XCTAssertEqual(next.assets.first?.assignmentKind, "TEMPORARY")
        XCTAssertEqual(next.revision, 6)

        XCTAssertThrowsError(try EnterpriseReferenceWire.parseResponse(
            Data(Vectors.response_other_membership.utf8), requestId: feedRequest, membershipId: feedMember,
            enrollmentId: feedEnrollment, sentCursor: nil, heldRevision: nil
        )) { XCTAssertEqual($0 as? EnterpriseProtocolFailure, .binding) }
        XCTAssertThrowsError(try EnterpriseReferenceWire.parseResponse(
            Data(Vectors.response_new_cursor_for_request_cursor.utf8), requestId: feedRequest, membershipId: feedMember,
            enrollmentId: feedEnrollment, sentCursor: heldCursor, heldRevision: 3
        )) { XCTAssertEqual($0 as? EnterpriseProtocolFailure, .binding) }
        XCTAssertThrowsError(try EnterpriseReferenceWire.parseResponse(
            Data(Vectors.response_unknown_top_level_field.utf8), requestId: feedRequest, membershipId: feedMember,
            enrollmentId: feedEnrollment, sentCursor: nil, heldRevision: nil
        )) { XCTAssertEqual($0 as? EnterpriseProtocolFailure, .invalid) }
        XCTAssertThrowsError(try EnterpriseReferenceWire.apply(prior: nil, response: delta, receivedAtMillis: 2_000))
    }

    // MARK: - Attachment session

    private func attachmentRequest() -> EnterpriseAttachmentWire.Request {
        let prefix = "7b0e2a54-0000-4000-8000-0000000415"
        return EnterpriseAttachmentWire.Request(
            requestId: prefix + "01", familyId: prefix + "02", accessToken: "synthetic-attachment-access",
            membershipId: prefix + "03", enrollmentId: prefix + "04", declarationId: prefix + "05", documentId: prefix + "06",
            linkId: prefix + "07", sha256: "f116935c2e2a3b64a8c45a692d7a1f45f9f837bc2751cea3a2c60ee9c59d2fc5", bytes: 1024,
            targetKind: "METER_READING", targetId: prefix + "08"
        )
    }

    func testAttachmentSessionMatchesTheSharedProfile() throws {
        let request = attachmentRequest()
        XCTAssertEqual(String(decoding: try EnterpriseAttachmentWire.requestBody(request), as: UTF8.self), Vectors.request_meter_reading)
        let uploading = try EnterpriseAttachmentWire.parseSession(Data(Vectors.response_uploading_capability.utf8), request: request)
        XCTAssertEqual(uploading.storage, .uploading)
        XCTAssertEqual(uploading.capability, "synthetic-upload-capability")
        let received = try EnterpriseAttachmentWire.parseSession(Data(Vectors.response_received.utf8), request: request)
        XCTAssertTrue(received.storage.holdsContent)
        XCTAssertNil(received.capability)
        XCTAssertThrowsError(try EnterpriseAttachmentWire.parseSession(Data(Vectors.response_capability_when_stored.utf8), request: request)) {
            XCTAssertEqual($0 as? EnterpriseProtocolFailure, .invalid)
        }
        XCTAssertThrowsError(try EnterpriseAttachmentWire.parseSession(Data(Vectors.response_expiry_trailing_zeros.utf8), request: request)) {
            XCTAssertEqual($0 as? EnterpriseProtocolFailure, .invalid)
        }
        XCTAssertThrowsError(try EnterpriseAttachmentWire.parseSession(Data(Vectors.response_revision_over_int64.utf8), request: request)) {
            XCTAssertEqual($0 as? EnterpriseProtocolFailure, .invalid)
        }
        XCTAssertThrowsError(try EnterpriseAttachmentWire.parseSession(Data(Vectors.response_foreign_document_id.utf8), request: request)) {
            XCTAssertEqual($0 as? EnterpriseProtocolFailure, .binding)
        }
    }

    func testCapabilityUploadRefusalNeverSettlesTheSignedReceipt() {
        let finalSessionRefusals = [(403, "FORBIDDEN"), (409, "CONFLICT"), (409, "OPERATION_ID_REUSED"), (422, "VALIDATION_FAILED")]
        for (status, code) in finalSessionRefusals {
            XCTAssertTrue(EnterpriseAttachmentWire.isFinalRefusal(status: status, code: code, source: .signedSession))
            XCTAssertFalse(EnterpriseAttachmentWire.isFinalRefusal(status: status, code: code, source: .capabilityUpload))
        }
        let transient = [(401, "UNAUTHENTICATED"), (403, "DEVICE_REVOKED"), (404, "NOT_FOUND"), (408, "TIMEOUT"),
                         (429, "RATE_LIMITED"), (503, "UNAVAILABLE")]
        for (status, code) in transient {
            XCTAssertFalse(EnterpriseAttachmentWire.isFinalRefusal(status: status, code: code, source: .signedSession))
            XCTAssertFalse(EnterpriseAttachmentWire.isFinalRefusal(status: status, code: code, source: .capabilityUpload))
        }
    }

    // MARK: - Vectors

    enum Vectors {
        static let request_no_cursor = #"{"access_token":"synthetic-reference-access","cursor":null,"enrollment_id":"7b0e2a54-0000-4000-8000-000000008613","family_id":"7b0e2a54-0000-4000-8000-000000008621","membership_id":"7b0e2a54-0000-4000-8000-000000008611","request_id":"7b0e2a54-0000-4000-8000-000000008620","schema_version":1}"#
        static let request_with_cursor = #"{"access_token":"synthetic-reference-access","cursor":"eyJ2IjoxLCJyIjozfQ.c3ludGhldGljLXNpZ25hdHVyZQ","enrollment_id":"7b0e2a54-0000-4000-8000-000000008613","family_id":"7b0e2a54-0000-4000-8000-000000008621","membership_id":"7b0e2a54-0000-4000-8000-000000008611","request_id":"7b0e2a54-0000-4000-8000-000000008620","schema_version":1}"#
        static let response_reset_new_cursor_one_asset = #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000008620","membership_id":"7b0e2a54-0000-4000-8000-000000008611","enrollment_id":"7b0e2a54-0000-4000-8000-000000008613","mode":"RESET","reason":"NEW_CURSOR","from_revision":null,"revision":1,"confirmed_at":"2026-11-15T12:00:00.123456Z","cursor":"eyJ2IjoxLCJyIjo1fQ.bmV4dC1zeW50aGV0aWMtc2lnbmF0dXJl","entity_types":["asset"],"changes":[{"revision":1,"entity_type":"asset","entity_id":"7b0e2a54-0000-4000-8000-000000008630","op":"UPSERT","asset":{"id":"7b0e2a54-0000-4000-8000-000000008630","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"Caminhão 12","fleet_number":"FROTA-12","plate":"ABC1D23","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[{"id":"7b0e2a54-0000-4000-8000-000000008730","kind":"ODOMETER","modulus":null,"segment_id":"7b0e2a54-0000-4000-8000-000000008731","installed_at":"2026-11-01T08:00:00Z"}],"assignment":{"kind":"PRIMARY","valid_to":null}}}]}"#
        static let response_delta_delete_and_upsert = #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000008620","membership_id":"7b0e2a54-0000-4000-8000-000000008611","enrollment_id":"7b0e2a54-0000-4000-8000-000000008613","mode":"DELTA","reason":null,"from_revision":3,"revision":6,"confirmed_at":"2026-11-15T12:00:00.123456Z","cursor":"eyJ2IjoxLCJyIjo1fQ.bmV4dC1zeW50aGV0aWMtc2lnbmF0dXJl","entity_types":["asset"],"changes":[{"revision":4,"entity_type":"asset","entity_id":"7b0e2a54-0000-4000-8000-000000008630","op":"DELETE","asset":null},{"revision":6,"entity_type":"asset","entity_id":"7b0e2a54-0000-4000-8000-000000008640","op":"UPSERT","asset":{"id":"7b0e2a54-0000-4000-8000-000000008640","revision":1,"kind":"MACHINE","usage_basis":"HOURS","state":"ACTIVE","name":"Escavadeira Ç 😀","fleet_number":"ESC-7","plate":"","catalog":"fuel-products-v1","products":["DIESEL_S10","DIESEL_S500"],"meters":[{"id":"7b0e2a54-0000-4000-8000-000000008741","kind":"HOUR_METER","modulus":100000,"segment_id":"7b0e2a54-0000-4000-8000-000000008742","installed_at":"2026-10-01T06:00:00.5Z"}],"assignment":{"kind":"TEMPORARY","valid_to":"2026-11-16T08:30:00.123456789Z"}}}]}"#
        static let response_other_membership = #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000008620","membership_id":"7b0e2a54-0000-4000-8000-000000008698","enrollment_id":"7b0e2a54-0000-4000-8000-000000008613","mode":"RESET","reason":"NEW_CURSOR","from_revision":null,"revision":1,"confirmed_at":"2026-11-15T12:00:00.123456Z","cursor":"eyJ2IjoxLCJyIjo1fQ.bmV4dC1zeW50aGV0aWMtc2lnbmF0dXJl","entity_types":["asset"],"changes":[{"revision":1,"entity_type":"asset","entity_id":"7b0e2a54-0000-4000-8000-000000008630","op":"UPSERT","asset":{"id":"7b0e2a54-0000-4000-8000-000000008630","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"Caminhão 12","fleet_number":"FROTA-12","plate":"ABC1D23","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[{"id":"7b0e2a54-0000-4000-8000-000000008730","kind":"ODOMETER","modulus":null,"segment_id":"7b0e2a54-0000-4000-8000-000000008731","installed_at":"2026-11-01T08:00:00Z"}],"assignment":{"kind":"PRIMARY","valid_to":null}}}]}"#
        static let response_new_cursor_for_request_cursor = #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000008620","membership_id":"7b0e2a54-0000-4000-8000-000000008611","enrollment_id":"7b0e2a54-0000-4000-8000-000000008613","mode":"RESET","reason":"NEW_CURSOR","from_revision":null,"revision":1,"confirmed_at":"2026-11-15T12:00:00.123456Z","cursor":"eyJ2IjoxLCJyIjo1fQ.bmV4dC1zeW50aGV0aWMtc2lnbmF0dXJl","entity_types":["asset"],"changes":[{"revision":1,"entity_type":"asset","entity_id":"7b0e2a54-0000-4000-8000-000000008630","op":"UPSERT","asset":{"id":"7b0e2a54-0000-4000-8000-000000008630","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"Caminhão 12","fleet_number":"FROTA-12","plate":"ABC1D23","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[{"id":"7b0e2a54-0000-4000-8000-000000008730","kind":"ODOMETER","modulus":null,"segment_id":"7b0e2a54-0000-4000-8000-000000008731","installed_at":"2026-11-01T08:00:00Z"}],"assignment":{"kind":"PRIMARY","valid_to":null}}}]}"#
        static let response_unknown_top_level_field = #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000008620","membership_id":"7b0e2a54-0000-4000-8000-000000008611","enrollment_id":"7b0e2a54-0000-4000-8000-000000008613","mode":"RESET","reason":"NEW_CURSOR","from_revision":null,"revision":1,"confirmed_at":"2026-11-15T12:00:00.123456Z","cursor":"eyJ2IjoxLCJyIjo1fQ.bmV4dC1zeW50aGV0aWMtc2lnbmF0dXJl","entity_types":["asset"],"changes":[{"revision":1,"entity_type":"asset","entity_id":"7b0e2a54-0000-4000-8000-000000008630","op":"UPSERT","asset":{"id":"7b0e2a54-0000-4000-8000-000000008630","revision":1,"kind":"VEHICLE","usage_basis":"DISTANCE","state":"ACTIVE","name":"Caminhão 12","fleet_number":"FROTA-12","plate":"ABC1D23","catalog":"fuel-products-v1","products":["DIESEL_S10"],"meters":[{"id":"7b0e2a54-0000-4000-8000-000000008730","kind":"ODOMETER","modulus":null,"segment_id":"7b0e2a54-0000-4000-8000-000000008731","installed_at":"2026-11-01T08:00:00Z"}],"assignment":{"kind":"PRIMARY","valid_to":null}}}],"extra":1}"#
        static let request_meter_reading = #"{"access_token":"synthetic-attachment-access","bytes":1024,"declaration_id":"7b0e2a54-0000-4000-8000-000000041505","document_id":"7b0e2a54-0000-4000-8000-000000041506","enrollment_id":"7b0e2a54-0000-4000-8000-000000041504","family_id":"7b0e2a54-0000-4000-8000-000000041502","kind":"RECEIPT","link_id":"7b0e2a54-0000-4000-8000-000000041507","media_type":"image/jpeg","membership_id":"7b0e2a54-0000-4000-8000-000000041503","request_id":"7b0e2a54-0000-4000-8000-000000041501","schema_version":1,"sha256":"f116935c2e2a3b64a8c45a692d7a1f45f9f837bc2751cea3a2c60ee9c59d2fc5","target_id":"7b0e2a54-0000-4000-8000-000000041508","target_kind":"METER_READING"}"#
        static let response_uploading_capability = #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000041501","membership_id":"7b0e2a54-0000-4000-8000-000000041503","enrollment_id":"7b0e2a54-0000-4000-8000-000000041504","document_id":"7b0e2a54-0000-4000-8000-000000041506","storage":"UPLOADING","storage_revision":1,"upload":{"capability":"synthetic-upload-capability","expires_at":"2026-11-15T12:05:00Z"}}"#
        static let response_received = #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000041501","membership_id":"7b0e2a54-0000-4000-8000-000000041503","enrollment_id":"7b0e2a54-0000-4000-8000-000000041504","document_id":"7b0e2a54-0000-4000-8000-000000041506","storage":"RECEIVED","storage_revision":2,"upload":null}"#
        static let response_capability_when_stored = #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000041501","membership_id":"7b0e2a54-0000-4000-8000-000000041503","enrollment_id":"7b0e2a54-0000-4000-8000-000000041504","document_id":"7b0e2a54-0000-4000-8000-000000041506","storage":"STORED","storage_revision":3,"upload":{"capability":"synthetic-upload-capability","expires_at":"2026-11-15T12:05:00Z"}}"#
        static let response_foreign_document_id = #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000041501","membership_id":"7b0e2a54-0000-4000-8000-000000041503","enrollment_id":"7b0e2a54-0000-4000-8000-000000041504","document_id":"7b0e2a54-0000-4000-8000-000000041596","storage":"UPLOADING","storage_revision":1,"upload":{"capability":"synthetic-upload-capability","expires_at":"2026-11-15T12:05:00Z"}}"#
        static let response_expiry_trailing_zeros = #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000041501","membership_id":"7b0e2a54-0000-4000-8000-000000041503","enrollment_id":"7b0e2a54-0000-4000-8000-000000041504","document_id":"7b0e2a54-0000-4000-8000-000000041506","storage":"UPLOADING","storage_revision":1,"upload":{"capability":"synthetic-upload-capability","expires_at":"2026-11-15T12:05:00.000Z"}}"#
        static let response_revision_over_int64 = #"{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000041501","membership_id":"7b0e2a54-0000-4000-8000-000000041503","enrollment_id":"7b0e2a54-0000-4000-8000-000000041504","document_id":"7b0e2a54-0000-4000-8000-000000041506","storage":"UPLOADING","storage_revision":9223372036854775808,"upload":{"capability":"synthetic-upload-capability","expires_at":"2026-11-15T12:05:00Z"}}"#
    }
}
