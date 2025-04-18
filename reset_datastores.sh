# Connect to Cassandra using cqlsh
cqlsh localhost 9042

# Truncate the game_sessions table
TRUNCATE game_system.game_sessions;

# Verify it's empty
SELECT * FROM game_system.game_sessions LIMIT 10;

# Exit cqlsh
EXIT;# Connect to Redis CLI


redis-cli

# Clear all keys in the current database
FLUSHDB

# Or clear all keys in all databases
FLUSHALL

# Verify leaderboards are gone
KEYS leaderboard:*

# Exit Redis CLI
exit

#Delete and recreate the topic
kafka-topics.sh --bootstrap-server localhost:9092 --delete --topic game-sessions

# Create the topic again with the same configuration
kafka-topics.sh --bootstrap-server localhost:9092 --create --topic game-sessions --partitions 1 --replication-factor 1

# Verify the topic is empty
kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic game-sessions --from-beginning

