-- Origin SQL:
ALTER TABLE test.events_local ON CLUSTER 'default_cluster' ADD COLUMN f1 String AFTER f0;


-- Format SQL:
ALTER TABLE test.events_local
ON CLUSTER 'default_cluster'
ADD COLUMN f1 STRING AFTER f0;