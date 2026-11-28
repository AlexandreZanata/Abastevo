-- Owned community read for decision notices (B-BR-NOTIF-02). The declared
-- directory join reads only the station's public display name.

-- name: DecisionNoticeFacts :one
SELECT o.contributor_ref, o.station_id, o.fuel_product,
    s.display_name AS station_name,
    (o.deleted_at IS NOT NULL)::boolean AS deleted
FROM community_observations o
JOIN directory_stations s ON s.id = o.station_id
WHERE o.id = @id;
