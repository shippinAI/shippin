-- Agent hub live updates: every hub_events row is announced on the hub_events channel with the
-- task's org, so every console replica can push it to that org's open boards (SSE).
CREATE OR REPLACE FUNCTION hub_events_notify() RETURNS trigger AS $$
BEGIN
  PERFORM pg_notify('hub_events', json_build_object('org_id', t.org_id, 'task_id', NEW.task_id,
    'event_id', NEW.id, 'kind', NEW.kind)::text)
  FROM hub_tasks t WHERE t.id = NEW.task_id;
  RETURN NEW;
END $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS hub_events_notify ON hub_events;
CREATE TRIGGER hub_events_notify AFTER INSERT ON hub_events FOR EACH ROW EXECUTE FUNCTION hub_events_notify();
