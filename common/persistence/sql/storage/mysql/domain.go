// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package mysql

import (
	"database/sql"
	"errors"

	"github.com/uber/cadence/common/persistence/sql/storage/sqldb"
)

const (
	shardID = 54321

	createDomainQry = `INSERT INTO domains (
		id,
		name,
		retention, 
		emit_metric,
		archival_bucket,
		archival_status,
		config_version,
		status, 
		description, 
		owner_email,
		failover_version, 
		is_global_domain,
		active_cluster_name, 
		clusters, 
		notification_version,
		failover_notification_version,
		data
		)
		VALUES(
		:id,
		:name,
		:retention, 
		:emit_metric,
		:archival_bucket,
		:archival_status,
		:config_version,
		:status, 
		:description, 
		:owner_email,
		:failover_version, 
		:is_global_domain,
		:active_cluster_name, 
		:clusters,
		:notification_version,
		:failover_notification_version,
		:data
		)`

	updateDomainQry = `UPDATE domains SET
		retention = :retention, 
		emit_metric = :emit_metric,
		archival_bucket = :archival_bucket,
		archival_status = :archival_status,
		config_version = :config_version,
		status = :status, 
		description = :description, 
		owner_email = :owner_email,
		failover_version = :failover_version, 
		active_cluster_name = :active_cluster_name,  
		clusters = :clusters,
		notification_version = :notification_version,
		failover_notification_version = :failover_notification_version,
		data = :data
		WHERE shard_id=54321 AND name = :name AND id = :id`

	getDomainPart = `SELECT
		id,
		retention, 
		emit_metric,
		archival_bucket,
		archival_status,
		config_version,
		name, 
		status, 
		description, 
		owner_email,
		failover_version, 
		is_global_domain,
		active_cluster_name, 
		clusters,
		notification_version,
		failover_notification_version,
		data FROM domains
`
	getDomainByIDQry   = getDomainPart + `WHERE shard_id=? AND id = ?`
	getDomainByNameQry = getDomainPart + `WHERE shard_id=? AND name = ?`

	deleteDomainByIDQry   = `DELETE FROM domains WHERE shard_id=? AND id = ?`
	deleteDomainByNameQry = `DELETE FROM domains WHERE shard_id=? AND name = ?`

	getDomainMetadataQry    = `SELECT notification_version FROM domain_metadata`
	lockDomainMetadataQry   = `SELECT notification_version FROM domain_metadata FOR UPDATE`
	updateDomainMetadataQry = `UPDATE domain_metadata SET notification_version = :notification_version + 1 
WHERE notification_version = :notification_version`

	listDomainsQry      = getDomainPart + ` WHERE shard_id=? ORDER BY id LIMIT ?`
	listDomainsRangeQry = getDomainPart + ` WHERE shard_id=? AND id > ? ORDER BY id LIMIT ?`
)

var errMissingArgs = errors.New("missing one or more args for API")

// InsertIntoDomain inserts a single row into domains table
func (mdb *DB) InsertIntoDomain(row *sqldb.DomainRow) (sql.Result, error) {
	return mdb.conn.NamedExec(createDomainQry, row)
}

// UpdateDomain updates a single row in domains table
func (mdb *DB) UpdateDomain(row *sqldb.DomainRow) (sql.Result, error) {
	return mdb.conn.NamedExec(updateDomainQry, row)
}

// SelectFromDomain reads one or more rows from domains table
func (mdb *DB) SelectFromDomain(filter *sqldb.DomainFilter) ([]sqldb.DomainRow, error) {
	switch {
	case filter.ID != nil || filter.Name != nil:
		return mdb.selectFromDomain(filter)
	case filter.PageSize != nil && *filter.PageSize > 0:
		return mdb.selectAllFromDomain(filter)
	default:
		return nil, errMissingArgs
	}
}

func (mdb *DB) selectFromDomain(filter *sqldb.DomainFilter) ([]sqldb.DomainRow, error) {
	var err error
	var row sqldb.DomainRow
	switch {
	case filter.ID != nil:
		err = mdb.conn.Get(&row, getDomainByIDQry, shardID, *filter.ID)
	case filter.Name != nil:
		err = mdb.conn.Get(&row, getDomainByNameQry, shardID, *filter.Name)
	}
	if err != nil {
		return nil, err
	}
	return []sqldb.DomainRow{row}, err
}

func (mdb *DB) selectAllFromDomain(filter *sqldb.DomainFilter) ([]sqldb.DomainRow, error) {
	var err error
	var rows []sqldb.DomainRow
	switch {
	case filter.GreaterThanID != nil:
		err = mdb.conn.Select(&rows, listDomainsRangeQry, shardID, *filter.GreaterThanID, *filter.PageSize)
	default:
		err = mdb.conn.Select(&rows, listDomainsQry, shardID, filter.PageSize)
	}
	return rows, err
}

// DeleteFromDomain deletes a single row in domains table
func (mdb *DB) DeleteFromDomain(filter *sqldb.DomainFilter) (sql.Result, error) {
	var err error
	var result sql.Result
	switch {
	case filter.ID != nil:
		result, err = mdb.conn.Exec(deleteDomainByIDQry, shardID, filter.ID)
	default:
		result, err = mdb.conn.Exec(deleteDomainByNameQry, shardID, filter.Name)
	}
	return result, err
}

// LockDomainMetadata acquires a write lock on a single row in domain_metadata table
func (mdb *DB) LockDomainMetadata() error {
	var row sqldb.DomainMetadataRow
	err := mdb.conn.Get(&row.NotificationVersion, lockDomainMetadataQry)
	return err
}

// SelectFromDomainMetadata reads a single row in domain_metadata table
func (mdb *DB) SelectFromDomainMetadata() (*sqldb.DomainMetadataRow, error) {
	var row sqldb.DomainMetadataRow
	err := mdb.conn.Get(&row.NotificationVersion, getDomainMetadataQry)
	return &row, err
}

// UpdateDomainMetadata updates a single row in domain_metadata table
func (mdb *DB) UpdateDomainMetadata(row *sqldb.DomainMetadataRow) (sql.Result, error) {
	return mdb.conn.NamedExec(updateDomainMetadataQry, row)
}
