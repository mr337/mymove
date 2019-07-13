ALTER TABLE duty_stations DROP CONSTRAINT duty_stations_address_id_fkey;

ALTER TABLE duty_stations
	ADD CONSTRAINT duty_stations_address_id_fkey FOREIGN KEY (address_id) REFERENCES public.addresses(id) ON DELETE CASCADE;
