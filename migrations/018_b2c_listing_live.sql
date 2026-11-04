-- Notify only committed real-inventory changes. No portal or timer is needed.
CREATE FUNCTION b2c_notify_listing_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.registered_by NOT LIKE 'sample:%' THEN
            PERFORM pg_notify('b2c_listings_changed', '');
        END IF;
    ELSIF TG_OP = 'DELETE' THEN
        IF OLD.registered_by NOT LIKE 'sample:%' THEN
            PERFORM pg_notify('b2c_listings_changed', '');
        END IF;
    ELSE
        IF (OLD.registered_by NOT LIKE 'sample:%' OR NEW.registered_by NOT LIKE 'sample:%')
           AND ROW(OLD.complex_name,OLD.type_name,OLD.asking_manwon,OLD.floor,OLD.lon,OLD.lat,OLD.status,OLD.registered_by,OLD.external_source)
               IS DISTINCT FROM ROW(NEW.complex_name,NEW.type_name,NEW.asking_manwon,NEW.floor,NEW.lon,NEW.lat,NEW.status,NEW.registered_by,NEW.external_source) THEN
            PERFORM pg_notify('b2c_listings_changed', '');
        END IF;
    END IF;
    RETURN NULL;
END;
$$;
CREATE TRIGGER b2c_listing_live AFTER INSERT OR UPDATE OR DELETE ON b2c_listings
    FOR EACH ROW EXECUTE FUNCTION b2c_notify_listing_change();
