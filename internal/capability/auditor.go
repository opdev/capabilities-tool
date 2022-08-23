package capability

import (
	"reflect"

	"github.com/opdev/opcap/internal/operator"
)

// Auditor interface represents the object running capability audits against operators
// It has methods to create a workqueue with all the package and audit requirements for a
// particular audit run
type Auditor interface {
	BuildWorkQueueByCatalog(catalogSource string, catalogSourceNamespace string, auditPlan []string) error
	RunAudits() error
}

// capAuditor implements Auditor
type capAuditor struct {

	// Workqueue holds capAudits in a buffered channel in order to execute them
	WorkQueue chan CapAudit
}

// BuildAuditorByCatalog creates a new Auditor with workqueue based on a selected catalog
func BuildAuditorByCatalog(catalogSource string, catalogSourceNamespace string, auditPlan []string, filter []string) (capAuditor, error) {

	var auditor capAuditor
	err := auditor.BuildWorkQueueByCatalog(catalogSource, catalogSourceNamespace, auditPlan, filter)
	if err != nil {
		logger.Fatalf("Unable to build workqueue err := %s", err.Error())
	}
	return auditor, nil
}

// BuildWorkQueueByCatalog fills in the auditor workqueue with all package information found in a specific catalog
func (capAuditor *capAuditor) BuildWorkQueueByCatalog(catalogSource string, catalogSourceNamespace string, auditPlan []string, filter []string) error {

	c, err := operator.NewOpCapClient()
	if err != nil {
		// if it doesn't load the client nothing can be done
		// log and panic
		logger.Panic("Error while creating OpCapClient: %w", err)
	}

	// Getting subscription data form the package manifests available in the selected catalog
	subscriptions, err := c.GetSubscriptionData(catalogSource, catalogSourceNamespace, filter)
	if err != nil {
		logger.Errorf("Error while getting bundles from CatalogSource %s: %w", catalogSource, err)
		return err
	}

	// build workqueue as buffered channel based subscriptionData list size
	capAuditor.WorkQueue = make(chan CapAudit, len(subscriptions))
	defer close(capAuditor.WorkQueue)

	// add capAudits to the workqueue
	for _, subscription := range subscriptions {

		capAudit, err := newCapAudit(c, subscription, auditPlan)
		if err != nil {
			logger.Debugf("Couldn't build capAudit for subscription %s", "Err:", err)
			return err
		}

		// load workqueue with capAudit
		capAuditor.WorkQueue <- capAudit
	}

	return nil
}

// RunAudits executes all selected functions in order for a given audit at a time
func (capAuditor *capAuditor) RunAudits() error {

	// read workqueue for audits
	for audit := range capAuditor.WorkQueue {

		// read a particular audit's auditPlan for functions
		// to be executed against operator
		for _, function := range audit.AuditPlan {

			// run function/method by name
			m := reflect.ValueOf(&audit).MethodByName(function)
			m.Call(nil)
		}

	}
	return nil
}
