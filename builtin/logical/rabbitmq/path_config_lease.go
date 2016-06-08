package rabbitmq

import (
	"time"

	"github.com/fatih/structs"
	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

func pathConfigLease(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "config/lease",
		Fields: map[string]*framework.FieldSchema{
			"ttl": &framework.FieldSchema{
				Type:        framework.TypeDurationSecond,
				Description: "Duration before which the issued credentials needs renewal",
			},
			"max_ttl": &framework.FieldSchema{
				Type:        framework.TypeDurationSecond,
				Description: `Duration after which the issued credentials shoulw not be allowed to be renewed`,
			},
		},

		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ReadOperation:   b.pathLeaseRead,
			logical.UpdateOperation: b.pathLeaseUpdate,
		},

		HelpSynopsis:    pathConfigLeaseHelpSyn,
		HelpDescription: pathConfigLeaseHelpDesc,
	}
}

// Sets the lease configuration parameters
func (b *backend) pathLeaseUpdate(req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	entry, err := logical.StorageEntryJSON("config/lease", &configLease{
		TTL:    time.Second * time.Duration(d.Get("ttl").(int)),
		MaxTTL: time.Second * time.Duration(d.Get("ttl").(int)),
	})
	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(entry); err != nil {
		return nil, err
	}

	return nil, nil
}

// Returns the lease configuration parameters
func (b *backend) pathLeaseRead(req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	lease, err := b.Lease(req.Storage)
	if err != nil {
		return nil, err
	}
	if lease == nil {
		return nil, nil
	}

	return &logical.Response{
		Data: structs.New(lease).Map(),
	}, nil
}

// Lease configuration information for the secrets issued by this backend
type configLease struct {
	TTL    time.Duration `json:"ttl" structs:"ttl" mapstructure:"ttl"`
	MaxTTL time.Duration `json:"max_ttl" structs:"max_ttl" mapstructure:"max_ttl"`
}

var pathConfigLeaseHelpSyn = "Configure the lease parameters for generated credentials"

var pathConfigLeaseHelpDesc = `
Sets the ttl and max_ttl values for the secrets to be issued by this backend.
Both ttl and max_ttl takes in an integet input as well as inputs like "1h".
`
