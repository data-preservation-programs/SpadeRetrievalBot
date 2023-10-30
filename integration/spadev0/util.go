package main

import (
	"context"
	"time"

	"github.com/data-preservation-programs/RetrievalBot/pkg/convert"
	"github.com/data-preservation-programs/RetrievalBot/pkg/env"
	"github.com/data-preservation-programs/RetrievalBot/pkg/requesterror"
	"github.com/data-preservation-programs/RetrievalBot/pkg/resolver"
	"github.com/data-preservation-programs/RetrievalBot/pkg/task"
	"github.com/filecoin-project/go-address"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func AddSpadeTasks(ctx context.Context, requester string, replicasToTest map[int][]Replica) error {
	var tasks []interface{}
	var results []interface{}

	// set up cache and resolvers
	providerCacheTTL := env.GetDuration(env.ProviderCacheTTL, 24*time.Hour)
	locationCacheTTL := env.GetDuration(env.LocationCacheTTL, 24*time.Hour)
	locationResolver := resolver.NewLocationResolver(env.GetRequiredString(env.IPInfoToken), locationCacheTTL)
	providerResolver, err := resolver.NewProviderResolver(
		env.GetString(env.LotusAPIUrl, "https://api.node.glif.io/rpc/v0"),
		env.GetString(env.LotusAPIToken, ""),
		providerCacheTTL)
	if err != nil {
		panic(err)
	}

	// Check public IP address
	ipInfo, err := resolver.GetPublicIPInfo(ctx, "", "")
	if err != nil {
		panic(err)
	}
	logger.With("ipinfo", ipInfo).Infof("Public IP info retrieved")

	address.CurrentNetwork = address.Mainnet

	// For each SPID, assemble retrieval tasks for it
	for spid, replicas := range replicasToTest {
		strSpid, err := address.NewIDAddress(uint64(spid))
		if err != nil {
			logger.Errorf("failed to convert spid to address: %d : %v", spid, err)
			continue
		}
		t, r := prepareTasksForSP(ctx, requester, strSpid, ipInfo, replicas, locationResolver, *providerResolver)

		//nolint:asasalint
		tasks = append(tasks, t...)
		//nolint:asasalint
		results = append(results, r...)
	}

	// Write resulting tasks and results to the DB
	taskClient, err := mongo.
		Connect(ctx, options.Client().ApplyURI(env.GetRequiredString(env.QueueMongoURI)))
	if err != nil {
		panic(err)
	}
	taskCollection := taskClient.
		Database(env.GetRequiredString(env.QueueMongoDatabase)).Collection("task_queue")

	if len(tasks) > 0 {
		_, err = taskCollection.InsertMany(ctx, tasks)
		if err != nil {
			return errors.Wrap(err, "failed to insert tasks")
		}
	}

	resultClient, err := mongo.Connect(ctx, options.Client().ApplyURI(env.GetRequiredString(env.ResultMongoURI)))
	if err != nil {
		panic(err)
	}
	resultCollection := resultClient.
		Database(env.GetRequiredString(env.ResultMongoDatabase)).
		Collection("task_result")

	if len(results) > 0 {
		_, err = resultCollection.InsertMany(ctx, results)
		if err != nil {
			return errors.Wrap(err, "failed to insert results")
		}
	}

	return nil
}

var moduleMetadataMap = map[task.ModuleName]map[string]string{
	task.GraphSync: {
		"assume_label":  "true",
		"retrieve_type": "root_block",
	},
	task.Bitswap: {
		"retrieve_type":      string(task.Spade),
		"max_traverse_depth": "3",
	},
	task.HTTP: {
		"retrieve_type": string(task.Piece),
		"retrieve_size": "1048576",
	},
}

func prepareTasksForSP(
	ctx context.Context,
	requester string,
	spid address.Address,
	ipInfo resolver.IPInfo,
	replicas []Replica,
	locationResolver resolver.LocationResolver,
	providerResolver resolver.ProviderResolver,
) (tasks []interface{}, results []interface{}) {
	providerInfo, err := providerResolver.ResolveProvider(ctx, spid.String())
	if err != nil {
		logger.With("provider", spid).
			Error("failed to resolve provider")
		return
	}

	location, err := locationResolver.ResolveMultiaddrsBytes(ctx, providerInfo.Multiaddrs)
	if err != nil {
		if errors.As(err, &requesterror.BogonIPError{}) ||
			errors.As(err, &requesterror.InvalidIPError{}) ||
			errors.As(err, &requesterror.HostLookupError{}) ||
			errors.As(err, &requesterror.NoValidMultiAddrError{}) {
			results = addErrorResults(requester, ipInfo, results, spid.String(), providerInfo, location,
				task.NoValidMultiAddrs, err.Error())
		} else {
			logger.With("provider", spid, "err", err).
				Error("failed to resolve provider location")
			return
		}
	}

	for _, document := range replicas {
		tasks = append(tasks, task.Task{
			Requester: requester,
			Module:    task.Bitswap,
			Metadata:  moduleMetadataMap[task.Bitswap],
			Provider: task.Provider{
				ID:         spid.String(),
				PeerID:     providerInfo.PeerId,
				Multiaddrs: convert.MultiaddrsBytesToStringArraySkippingError(providerInfo.Multiaddrs),
				City:       location.City,
				Region:     location.Region,
				Country:    location.Country,
				Continent:  location.Continent,
			},
			Content: task.Content{
				CID: document.OptionalDagRoot,
			},
			CreatedAt: time.Now().UTC(),
			Timeout:   env.GetDuration(env.SpadeIntegrationTaskTimeout, 15*time.Second),
		})
	}

	for _, document := range replicas {
		tasks = append(tasks, task.Task{
			Requester: requester,
			Module:    task.HTTP,
			Metadata:  moduleMetadataMap[task.HTTP],
			Provider: task.Provider{
				ID:         document.PieceCID,
				PeerID:     providerInfo.PeerId,
				Multiaddrs: convert.MultiaddrsBytesToStringArraySkippingError(providerInfo.Multiaddrs),
				City:       location.City,
				Region:     location.Region,
				Country:    location.Country,
				Continent:  location.Continent,
			},
			Content: task.Content{
				CID: document.PieceCID,
			},
			CreatedAt: time.Now().UTC(),
			Timeout:   env.GetDuration(env.SpadeIntegrationTaskTimeout, 15*time.Second),
		})
	}

	return tasks, results
}

func addErrorResults(
	requester string,
	ipInfo resolver.IPInfo,
	results []interface{},
	spid string,
	providerInfo resolver.MinerInfo,
	location resolver.IPInfo,
	errorCode task.ErrorCode,
	errorMessage string,
) []interface{} {
	for module, metadata := range moduleMetadataMap {
		newMetadata := make(map[string]string)
		for k, v := range metadata {
			newMetadata[k] = v
		}
		newMetadata["client"] = string(module)
		results = append(results, task.Result{
			Task: task.Task{
				Requester: requester,
				Module:    "spadev0",
				Metadata:  newMetadata,
				Provider: task.Provider{
					ID:         spid,
					PeerID:     providerInfo.PeerId,
					Multiaddrs: convert.MultiaddrsBytesToStringArraySkippingError(providerInfo.Multiaddrs),
					City:       location.City,
					Region:     location.Region,
					Country:    location.Country,
					Continent:  location.Continent,
				},
				CreatedAt: time.Now().UTC(),
				Timeout:   env.GetDuration(env.SpadeIntegrationTaskTimeout, 15*time.Second)},
			Retriever: task.Retriever{
				PublicIP:  ipInfo.IP,
				City:      ipInfo.City,
				Region:    ipInfo.Region,
				Country:   ipInfo.Country,
				Continent: ipInfo.Continent,
				ASN:       ipInfo.ASN,
				ISP:       ipInfo.ISP,
				Latitude:  ipInfo.Latitude,
				Longitude: ipInfo.Longitude,
			},
			Result: task.RetrievalResult{
				Success:      false,
				ErrorCode:    errorCode,
				ErrorMessage: errorMessage,
				TTFB:         0,
				Speed:        0,
				Duration:     0,
				Downloaded:   0,
			},
			CreatedAt: time.Now().UTC(),
		})
	}
	return results
}
